//go:build integration

package features

import (
	"os"
	"testing"

	"nac-service-media/features/steps"

	"github.com/cucumber/godog"
	"github.com/cucumber/godog/colors"
)

func TestFeatures(t *testing.T) {
	opts := godog.Options{
		Format:   "pretty",
		Output:   colors.Colored(os.Stdout),
		Paths:    []string{"./"},
		TestingT: t,
	}

	suite := godog.TestSuite{
		ScenarioInitializer: initializeScenarios,
		Options:             &opts,
	}

	if suite.Run() != 0 {
		t.Fatal("non-zero status returned, failed to run feature tests")
	}
}

func initializeScenarios(ctx *godog.ScenarioContext) {
	steps.InitializeConfigScenario(ctx)
	steps.InitializeSetupScenario(ctx)
	steps.InitializeTrimScenario(ctx)
}
