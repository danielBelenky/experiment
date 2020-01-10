package handler

import (
	"fmt"
	"io/ioutil"
	"path/filepath"
	"reflect"
	"sigs.k8s.io/yaml"
	"strings"

	"github.com/sirupsen/logrus"

	//git "github.com/danielBelenky/experiment/pkg/git_utils"
	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/git"
	gitv2 "k8s.io/test-infra/prow/git/v2"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/pjutil"
)

// Map from a the path the config was found it to the config itself
type configPathMap map[string]*config.Config

// HandlePullRequestEvent handles new pull request events
func HandlePullRequestEvent(
	event *github.PullRequestEvent, prowConfigPath, jobConfigPatterns, wd string) {
	// Make sure we do not crash the program
	defer logOnPanic()

	if !shouldPerformOnEvent(event) {
		logrus.Infof("nothing to do on pr %d", event.PullRequest.Number)
		return
	}

	gitClient, err := git.NewClient()
	if err != nil {
		logrus.WithError(err).Fatal("could not initialize git client")
		return
	}

	// TODO: improve performance with caching
	defer gitClient.Clean()

	// Disable authentication since we don't really need it
	gitClient.SetCredentials("", func() []byte { return []byte{}})

	org, name, err := gitv2.OrgRepo(event.Repo.FullName)
	if err != nil {
		logrus.WithError(err).Fatal("could not extract repo org and name")
		return
	}
	repo, err := gitClient.Clone(org, name)
	if err != nil {
		logrus.WithError(err).Fatal("could not clone repo")
		return
	}

	err = repo.CheckoutPullRequest(event.PullRequest.Number)
	if err != nil {
		logrus.WithError(err).Fatal("could not fetch pull request")
		return
	}

	// TODO: handle changes to config.yaml and presets
	modifiedJobConfigs, err := getModifiedConfigs(repo, event, jobConfigPatterns)
	if err != nil {
		logrus.WithError(err).Fatal("could not get the modified configs")
		return
	}

	if len(modifiedJobConfigs) == 0 {
		logrus.Infoln("no job configs were modified - nothing to do")
		return
	}

	modifiedConfigs, err := loadConfigsAtRef(repo, event.PullRequest.Head.SHA, prowConfigPath, modifiedJobConfigs)
	if err != nil {
		logrus.WithError(err).Fatal("could not load the modified configs from pull request's head")
		return
	}

	originalConfigs, err := loadConfigsAtRef(repo, event.PullRequest.Base.SHA, prowConfigPath, modifiedJobConfigs)
	if err != nil {
		logrus.WithError(err).Fatal("could not load the modified configs from pull request's base")
		return
	}

	squashedConfigs := squashConfigs(originalConfigs, modifiedConfigs)

	prowJobs := generateProwJobs(squashedConfigs, event)
	writeJobs(prowJobs)
}

// Get the modified job configs from the repo
func getModifiedConfigs(repo *git.Repo, event *github.PullRequestEvent, pattern string) ([]string, error) {
	changes, err := repo.Diff(event.PullRequest.Head.SHA, event.PullRequest.Base.SHA)
	if err != nil {
		return nil, err
	}
	return filterByPattern(changes, pattern)
}

// Filter the input array/slice by the given pattern.
// The pattern has to be a shell file name pattern.
// https://golang.org/pkg/path/filepath/#Match
func filterByPattern(input []string, pattern string) ([]string, error) {
	var out []string
	for _, s := range input {
		match, err := filepath.Match(pattern, s)
		if err != nil {
			// The only possible error is ErrorBadPattern
			return nil, err
		}
		if match {
			out = append(out, s)
		}
	}
	return out, nil
}

// Determine if we should perform on the given event.
func shouldPerformOnEvent(event *github.PullRequestEvent) bool {
	// See https://developer.github.com/v3/activity/events/types/#pullrequestevent
	// for details
	switch event.Action {
	case "opened":
		return true
	case "edited":
		return true
	case "synchronize":
		return true
	default:
		return false
	}
}

// Load job configs but checkout before
func loadConfigsAtRef(
	repo *git.Repo, ref, prowConfigPath string, jobConfigPaths []string) (configPathMap, error) {
	if err := repo.Checkout(ref); err != nil {
		return nil, err
	}

	return loadConfigs(repo.Directory(), prowConfigPath, jobConfigPaths), nil
}

// Squash original and modified configs at the same path, returning an array of configs
// that contain only new and modified job configs.
func squashConfigs(originalConfigs, modifiedConfigs configPathMap) []*config.Config {
	var configs []*config.Config
	for path, headConfig := range modifiedConfigs {
		baseConfig, exists := originalConfigs[path]
		if !exists {
			// new config
			configs = append(configs, headConfig)
			continue
		}
		squashedConfig := new(config.Config)
		squashedConfig.PresubmitsStatic = squashPresubmitsStatic(
			baseConfig.PresubmitsStatic, headConfig.PresubmitsStatic)
		configs = append(configs, squashedConfig)
	}
	return configs
}

// Squash PresubmitsStatic config
func squashPresubmitsStatic(originalPresubmits,
	modifiedPresubmits map[string][]config.Presubmit) map[string][]config.Presubmit {
	squashedPresubmitConfigs := make(map[string][]config.Presubmit)
	for repo, headPresubmits := range modifiedPresubmits {
		basePresubmits, exists := originalPresubmits[repo]
		if !exists {
			// new presubmits
			squashedPresubmitConfigs[repo] = headPresubmits
			continue
		}
		squashedPresubmitConfigs[repo] = squashPresubmits(basePresubmits, headPresubmits)
	}
	return squashedPresubmitConfigs
}

// Given two arrays of presubmits, return a new array containing only the modified and new ones.
func squashPresubmits(originalPresubmits,
	modifiedPresubmits []config.Presubmit) []config.Presubmit {
	var squashedPresubmits []config.Presubmit
	for _, headPresubmit := range modifiedPresubmits {
		presubmitIsNew := true
		for _, basePresubmit := range originalPresubmits {
			if basePresubmit.Name != headPresubmit.Name {
				continue
			}
			presubmitIsNew = false
			if reflect.DeepEqual(headPresubmit.Spec, basePresubmit.Spec) {
				continue
			}
			squashedPresubmits = append(squashedPresubmits, headPresubmit)
		}
		if presubmitIsNew {
			squashedPresubmits = append(squashedPresubmits, headPresubmit)
		}
	}
	return squashedPresubmits
}

func loadConfigs(root, prowConfPath string, jobConfPaths []string) configPathMap {
	configPaths := make(configPathMap)
	for _, jobConfPath := range jobConfPaths {
		prowConfInRepo := filepath.Join(root, prowConfPath)
		jobConfInRepo := filepath.Join(root, jobConfPath)
		conf, err := config.Load(prowConfInRepo, jobConfInRepo)
		if err != nil {
			logrus.WithError(err).Warnf("could not load config %s", jobConfInRepo)
			continue
		}
		configPaths[jobConfPath] = conf
	}
	return configPaths
}

func writeJobs(jobs []prowapi.ProwJob) {
	for _, job := range jobs {
		y, _ := yaml.Marshal(&job)
		filename := fmt.Sprintf("/tmp/%s.yaml", job.GetName())
		err := ioutil.WriteFile(filename, y, 0644)
		if err != nil {
			logrus.Errorln(err.Error())
		}
	}
}

// Generate the ProwJobs from the configs and the PR event.
func generateProwJobs(
	configs []*config.Config, pre *github.PullRequestEvent) []prowapi.ProwJob {
	var jobs []prowapi.ProwJob

	logrus.Infoln("Will process jobs from ", len(configs), "configs")
	for _, conf := range configs {
		jobs = append(jobs, generatePresubmits(conf, pre)...)
	}

	return jobs
}

func generatePresubmits(conf *config.Config, pre *github.PullRequestEvent) []prowapi.ProwJob {
	var jobs []prowapi.ProwJob

	for repo, presubmits := range conf.PresubmitsStatic {
		for _, presubmit := range presubmits {
			pj := pjutil.NewPresubmit(
				pre.PullRequest, pre.PullRequest.Base.SHA, presubmit, pre.GUID)
			addRepoRef(&pj, repo)
			logrus.Infof("Adding job: %s", pj.Name)
			jobs = append(jobs, pj)
		}
	}

	return jobs
}

// Add ref of the original repo which we want to check.
func addRepoRef(prowJob *prowapi.ProwJob, repo string) {
	// pj.Spec.Refs[0] is being notified by reportlib.
	shouldSetrepo := true
	for _, ref := range prowJob.Spec.ExtraRefs {
		// If a repo was set on one
		if ref.WorkDir {
			shouldSetrepo = false
			break
		}
	}
	repoSplit := strings.Split(repo, "/")
	org, name := repoSplit[0], repoSplit[1]
	ref := prowapi.Refs{
		Org:        org,
		Repo:       name,
		RepoLink:   fmt.Sprintf("https://github.com/%s", repo),
		BaseRef:    "refs/heads/master",
		WorkDir:    shouldSetrepo,
		CloneDepth: 50,
	}
	prowJob.Spec.ExtraRefs = append(prowJob.Spec.ExtraRefs, ref)
}

func logOnPanic() {
	if r := recover(); r != nil {
		logrus.Errorln(r)
	}
}
