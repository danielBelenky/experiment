package handler

import (
	"github.com/stretchr/testify/assert"
	"testing"

	"k8s.io/test-infra/prow/config"
)

func TestSquashConfigs(t *testing.T) {
	originalConfigs := configPathMap{
		"foo/path": &config.Config{
			JobConfig: config.JobConfig{
				PresubmitsStatic: map[string][]config.Presubmit{
					"foo/bar": {
						{
							JobBase: config.JobBase{
								Name: "dont-touch",
							},
						},
						{
							JobBase: config.JobBase{
								Name:           "modify-something",
								MaxConcurrency: 1,
							},
						},
					},
					"foo/baz": {
						{
							JobBase: config.JobBase{
								Name: "dont-touch",
							},
						},
					},
				},
			},
		},
	}
	modifiedConfigs := configPathMap{
		"foo/path": &config.Config{
			JobConfig: config.JobConfig{
				PresubmitsStatic: map[string][]config.Presubmit{
					"foo/bar": {
						{
							JobBase: config.JobBase{
								Name: "dont-touch",
							},
						},
						{
							JobBase: config.JobBase{
								Name:           "modify-something",
								MaxConcurrency: 2,
							},
						},
					},
					"foo/baz": {
						{
							JobBase: config.JobBase{
								Name: "dont-touch",
							},
						},
						{
							JobBase: config.JobBase{
								Name:           "new-presubmit",
								MaxConcurrency: 1,
							},
						},
					},
				},
			},
		},
	}
	expectedConfigs := []*config.Config{
		{
			JobConfig: config.JobConfig{
				PresubmitsStatic: map[string][]config.Presubmit{
					"foo/bar": {
						{
							JobBase: config.JobBase{
								Name:           "modify-something",
								MaxConcurrency: 2,
							},
						},
					},
					"foo/baz": {
						{
							JobBase: config.JobBase{
								Name:           "new-presubmit",
								MaxConcurrency: 1,
							},
						},
					},
				},
			},
		},
	}
	res := squashConfigs(originalConfigs, modifiedConfigs)
	assert.Equal(t, expectedConfigs, res)
}

func TestSquashPresubmitConfigs(t *testing.T) {
	originalConfigs := map[string][]config.Presubmit{
		"foo/bar": {
			{
				JobBase: config.JobBase{
					Name: "dont-touch",
				},
			},
			{
				JobBase: config.JobBase{
					Name:           "modify-something",
					MaxConcurrency: 1,
				},
			},
		},
		"foo/baz": {
			{
				JobBase: config.JobBase{
					Name: "dont-touch",
				},
			},
		},
	}
	modifiedConfigs := map[string][]config.Presubmit{
		"foo/bar": {
			{
				JobBase: config.JobBase{
					Name: "dont-touch",
				},
			},
			{
				JobBase: config.JobBase{
					Name: "modify-something",
					// Modified MaxConcurrency
					MaxConcurrency: 2,
				},
			},
		},
		"foo/baz": {
			{
				JobBase: config.JobBase{
					Name: "dont-touch",
				},
			},
			{
				JobBase: config.JobBase{
					Name: "new-presubmit",
				},
			},
		},
	}
	expectedConfigs := map[string][]config.Presubmit{
		"foo/bar": {
			{
				JobBase: config.JobBase{
					Name: "modify-something",
					// Modified MaxConcurrency
					MaxConcurrency: 2,
				},
			},
		},
		"foo/baz": {
			{
				JobBase: config.JobBase{
					Name: "new-presubmit",
				},
			},
		},
	}

	res := squashPresubmitsStatic(originalConfigs, modifiedConfigs)
	assert.Equal(t, res, expectedConfigs)
}

func TestSquashPresubmits(t *testing.T) {
	originalPresubmits := []config.Presubmit{
		{
			JobBase: config.JobBase{
				Name: "dont-touch",
			},
		},
		{
			JobBase: config.JobBase{
				Name:           "modify-something",
				MaxConcurrency: 1,
			},
		},
	}
	newPresubmits := []config.Presubmit{
		{
			JobBase: config.JobBase{
				Name: "dont-touch",
			},
		},
		{
			JobBase: config.JobBase{
				Name: "modify-something",
				// modified MaxConcurrency
				MaxConcurrency: 2,
			},
		},
		{
			JobBase: config.JobBase{
				Name: "new-job",
			},
		},
	}
	expectedPresubmits := []config.Presubmit{
		{
			JobBase: config.JobBase{
				Name: "modify-something",
				// modified MaxConcurrency
				MaxConcurrency: 2,
			},
		},
		{
			JobBase: config.JobBase{
				Name: "new-job",
			},
		},
	}
	res := squashPresubmits(originalPresubmits, newPresubmits)
	assert.Equal(t, expectedPresubmits, res)
}
