module github.com/danielBelenky/experiment

go 1.13

replace k8s.io/client-go => k8s.io/client-go v0.0.0-20190918200256-06eb1244587a

require (
	github.com/sirupsen/logrus v1.4.2
	github.com/stretchr/testify v1.4.0
	k8s.io/test-infra v0.0.0-20200110061235-48932aa5dd4c
	sigs.k8s.io/yaml v1.1.0
)
