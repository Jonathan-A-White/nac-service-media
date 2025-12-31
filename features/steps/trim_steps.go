//go:build integration

package steps

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"nac-service-media/cmd"
	"nac-service-media/domain/video"

	"github.com/cucumber/godog"
)

// mockTrimmer records calls to Trim for verification
type mockTrimmer struct {
	calls      []trimCall
	shouldFail bool
	failError  error
}

type trimCall struct {
	req        *video.TrimRequest
	outputPath string
	args       []string
}

func (m *mockTrimmer) Trim(ctx context.Context, req *video.TrimRequest, outputPath string) error {
	if m.shouldFail {
		return m.failError
	}
	m.calls = append(m.calls, trimCall{
		req:        req,
		outputPath: outputPath,
		args: []string{
			"-i", req.SourcePath,
			"-ss", req.Start.String(),
			"-to", req.End.String(),
			"-c", "copy",
			"-y", outputPath,
		},
	})
	return nil
}

// mockFileChecker simulates file existence
type mockFileChecker struct {
	existingFiles map[string]bool
}

func (m *mockFileChecker) Exists(path string) bool {
	return m.existingFiles[path]
}

// trimContext holds test state for trim scenarios
type trimContext struct {
	sourcePath  string
	outputDir   string
	startTime   string
	endTime     string
	trimmer     *mockTrimmer
	fileChecker *mockFileChecker
	output      *bytes.Buffer
	err         error
	resultPath  string
}

// SharedTrimContext is reset before each scenario via Before hook
var SharedTrimContext *trimContext

func getTrimContext() *trimContext {
	return SharedTrimContext
}

func InitializeTrimScenario(ctx *godog.ScenarioContext) {
	ctx.Before(func(c context.Context, sc *godog.Scenario) (context.Context, error) {
		SharedTrimContext = &trimContext{
			trimmer: &mockTrimmer{},
			fileChecker: &mockFileChecker{
				existingFiles: make(map[string]bool),
			},
			output: &bytes.Buffer{},
		}
		return c, nil
	})

	ctx.After(func(c context.Context, sc *godog.Scenario, err error) (context.Context, error) {
		SharedTrimContext = nil
		return c, nil
	})

	ctx.Step(`^the trimmed output directory is "([^"]*)"$`, theTrimmedOutputDirectoryIs)
	ctx.Step(`^a source video at "([^"]*)"$`, aSourceVideoAt)
	ctx.Step(`^no source video exists at "([^"]*)"$`, noSourceVideoExistsAt)
	ctx.Step(`^I trim the video from "([^"]*)" to "([^"]*)"$`, iTrimTheVideoFromTo)
	ctx.Step(`^I attempt to trim with start time "([^"]*)"$`, iAttemptToTrimWithStartTime)
	ctx.Step(`^I attempt to trim from "([^"]*)" to "([^"]*)"$`, iAttemptToTrimFromTo)
	ctx.Step(`^the output file should be "([^"]*)"$`, theOutputFileShouldBe)
	ctx.Step(`^ffmpeg should have been called with arguments:$`, ffmpegShouldHaveBeenCalledWithArguments)
	ctx.Step(`^I should receive an error about invalid timestamp format$`, iShouldReceiveAnErrorAboutInvalidTimestampFormat)
	ctx.Step(`^I should receive an error about end time before start time$`, iShouldReceiveAnErrorAboutEndTimeBeforeStartTime)
	ctx.Step(`^I should receive an error about missing source file$`, iShouldReceiveAnErrorAboutMissingSourceFile)
	ctx.Step(`^I should receive an error about invalid source filename$`, iShouldReceiveAnErrorAboutInvalidSourceFilename)
}

func theTrimmedOutputDirectoryIs(dir string) error {
	t := getTrimContext()
	t.outputDir = dir
	return nil
}

func aSourceVideoAt(path string) error {
	t := getTrimContext()
	t.sourcePath = path
	t.fileChecker.existingFiles[path] = true
	return nil
}

func noSourceVideoExistsAt(path string) error {
	t := getTrimContext()
	t.sourcePath = path
	t.fileChecker.existingFiles[path] = false
	return nil
}

func iTrimTheVideoFromTo(start, end string) error {
	t := getTrimContext()
	t.startTime = start
	t.endTime = end

	t.err = cmd.RunTrimWithDependencies(
		context.Background(),
		t.trimmer,
		t.fileChecker,
		t.outputDir,
		t.sourcePath,
		t.startTime,
		t.endTime,
		t.output,
	)

	if t.err != nil {
		return fmt.Errorf("unexpected error: %v", t.err)
	}

	// Capture result path from mock calls
	if len(t.trimmer.calls) > 0 {
		t.resultPath = t.trimmer.calls[0].outputPath
	}
	return nil
}

func iAttemptToTrimWithStartTime(start string) error {
	t := getTrimContext()
	t.startTime = start
	t.endTime = "01:00:00" // Valid end time

	t.err = cmd.RunTrimWithDependencies(
		context.Background(),
		t.trimmer,
		t.fileChecker,
		t.outputDir,
		t.sourcePath,
		t.startTime,
		t.endTime,
		t.output,
	)
	return nil
}

func iAttemptToTrimFromTo(start, end string) error {
	t := getTrimContext()
	t.startTime = start
	t.endTime = end

	t.err = cmd.RunTrimWithDependencies(
		context.Background(),
		t.trimmer,
		t.fileChecker,
		t.outputDir,
		t.sourcePath,
		t.startTime,
		t.endTime,
		t.output,
	)
	return nil
}

func theOutputFileShouldBe(expected string) error {
	t := getTrimContext()
	if t.resultPath != expected {
		return fmt.Errorf("expected output path %q, got %q", expected, t.resultPath)
	}
	return nil
}

func ffmpegShouldHaveBeenCalledWithArguments(table *godog.Table) error {
	t := getTrimContext()
	if len(t.trimmer.calls) == 0 {
		return fmt.Errorf("ffmpeg was not called")
	}

	call := t.trimmer.calls[0]

	for i, row := range table.Rows {
		if i == 0 {
			continue // Skip header row
		}
		expectedArg := row.Cells[0].Value
		found := false
		for _, arg := range call.args {
			if arg == expectedArg {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("expected argument %q not found in ffmpeg call: %v", expectedArg, call.args)
		}
	}
	return nil
}

func iShouldReceiveAnErrorAboutInvalidTimestampFormat() error {
	t := getTrimContext()
	if t.err == nil {
		return fmt.Errorf("expected an error but got none")
	}
	if !strings.Contains(t.err.Error(), "invalid timestamp format") {
		return fmt.Errorf("expected error about invalid timestamp format, got: %v", t.err)
	}
	return nil
}

func iShouldReceiveAnErrorAboutEndTimeBeforeStartTime() error {
	t := getTrimContext()
	if t.err == nil {
		return fmt.Errorf("expected an error but got none")
	}
	if !strings.Contains(t.err.Error(), "must be after start time") {
		return fmt.Errorf("expected error about end time before start time, got: %v", t.err)
	}
	return nil
}

func iShouldReceiveAnErrorAboutMissingSourceFile() error {
	t := getTrimContext()
	if t.err == nil {
		return fmt.Errorf("expected an error but got none")
	}
	if !strings.Contains(t.err.Error(), "does not exist") {
		return fmt.Errorf("expected error about missing source file, got: %v", t.err)
	}
	return nil
}

func iShouldReceiveAnErrorAboutInvalidSourceFilename() error {
	t := getTrimContext()
	if t.err == nil {
		return fmt.Errorf("expected an error but got none")
	}
	if !strings.Contains(t.err.Error(), "does not match expected format") {
		return fmt.Errorf("expected error about invalid source filename, got: %v", t.err)
	}
	return nil
}
