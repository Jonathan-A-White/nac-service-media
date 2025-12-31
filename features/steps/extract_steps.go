//go:build integration

package steps

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"time"

	"nac-service-media/cmd"
	"nac-service-media/domain/video"

	"github.com/cucumber/godog"
)

// mockExtractor records calls to Extract for verification
type mockExtractor struct {
	calls      []extractCall
	shouldFail bool
	failError  error
}

type extractCall struct {
	req        *video.AudioExtractionRequest
	outputPath string
	args       []string
}

func (m *mockExtractor) Extract(ctx context.Context, req *video.AudioExtractionRequest, outputPath string) error {
	if m.shouldFail {
		return m.failError
	}
	m.calls = append(m.calls, extractCall{
		req:        req,
		outputPath: outputPath,
		args: []string{
			"-i", req.SourceVideoPath,
			"-vn",
			"-acodec", "libmp3lame",
			"-ab", req.Bitrate,
			"-y", outputPath,
		},
	})
	return nil
}

// extractContext holds test state for extract scenarios
type extractContext struct {
	sourcePath     string
	outputDir      string
	bitrate        string
	serviceDate    time.Time
	extractor      *mockExtractor
	fileChecker    *mockFileChecker
	output         *bytes.Buffer
	err            error
	resultPath     string
}

// SharedExtractContext is reset before each scenario via Before hook
var SharedExtractContext *extractContext

func getExtractContext() *extractContext {
	return SharedExtractContext
}

func InitializeExtractScenario(ctx *godog.ScenarioContext) {
	ctx.Before(func(c context.Context, sc *godog.Scenario) (context.Context, error) {
		SharedExtractContext = &extractContext{
			extractor: &mockExtractor{},
			fileChecker: &mockFileChecker{
				existingFiles: make(map[string]bool),
			},
			output:  &bytes.Buffer{},
			bitrate: video.DefaultAudioBitrate,
		}
		return c, nil
	})

	ctx.After(func(c context.Context, sc *godog.Scenario, err error) (context.Context, error) {
		SharedExtractContext = nil
		return c, nil
	})

	ctx.Step(`^the audio output directory is "([^"]*)"$`, theAudioOutputDirectoryIs)
	ctx.Step(`^the default audio bitrate is "([^"]*)"$`, theDefaultAudioBitrateIs)
	ctx.Step(`^a trimmed video at "([^"]*)"$`, aTrimmedVideoAt)
	ctx.Step(`^no trimmed video exists at "([^"]*)"$`, noTrimmedVideoExistsAt)
	ctx.Step(`^I extract audio for service date "([^"]*)"$`, iExtractAudioForServiceDate)
	ctx.Step(`^I extract audio with bitrate "([^"]*)" for service date "([^"]*)"$`, iExtractAudioWithBitrateForServiceDate)
	ctx.Step(`^I attempt to extract audio for service date "([^"]*)"$`, iAttemptToExtractAudioForServiceDate)
	ctx.Step(`^the audio output file should be "([^"]*)"$`, theAudioOutputFileShouldBe)
	ctx.Step(`^ffmpeg should have been called with audio arguments:$`, ffmpegShouldHaveBeenCalledWithAudioArguments)
	ctx.Step(`^I should receive an error about missing source video$`, iShouldReceiveAnErrorAboutMissingSourceVideo)
}

func theAudioOutputDirectoryIs(dir string) error {
	e := getExtractContext()
	e.outputDir = dir
	return nil
}

func theDefaultAudioBitrateIs(bitrate string) error {
	e := getExtractContext()
	e.bitrate = bitrate
	return nil
}

func aTrimmedVideoAt(path string) error {
	e := getExtractContext()
	e.sourcePath = path
	e.fileChecker.existingFiles[path] = true
	return nil
}

func noTrimmedVideoExistsAt(path string) error {
	e := getExtractContext()
	e.sourcePath = path
	e.fileChecker.existingFiles[path] = false
	return nil
}

func iExtractAudioForServiceDate(dateStr string) error {
	e := getExtractContext()

	serviceDate, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return fmt.Errorf("invalid service date format: %w", err)
	}
	e.serviceDate = serviceDate

	e.err = cmd.RunExtractAudioWithDependencies(
		context.Background(),
		e.extractor,
		e.fileChecker,
		e.outputDir,
		e.bitrate,
		e.sourcePath,
		e.serviceDate,
		e.output,
	)

	if e.err != nil {
		return fmt.Errorf("unexpected error: %v", e.err)
	}

	// Capture result path from mock calls
	if len(e.extractor.calls) > 0 {
		e.resultPath = e.extractor.calls[0].outputPath
	}
	return nil
}

func iExtractAudioWithBitrateForServiceDate(bitrate, dateStr string) error {
	e := getExtractContext()
	e.bitrate = bitrate

	serviceDate, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return fmt.Errorf("invalid service date format: %w", err)
	}
	e.serviceDate = serviceDate

	e.err = cmd.RunExtractAudioWithDependencies(
		context.Background(),
		e.extractor,
		e.fileChecker,
		e.outputDir,
		e.bitrate,
		e.sourcePath,
		e.serviceDate,
		e.output,
	)

	if e.err != nil {
		return fmt.Errorf("unexpected error: %v", e.err)
	}

	// Capture result path from mock calls
	if len(e.extractor.calls) > 0 {
		e.resultPath = e.extractor.calls[0].outputPath
	}
	return nil
}

func iAttemptToExtractAudioForServiceDate(dateStr string) error {
	e := getExtractContext()

	serviceDate, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return fmt.Errorf("invalid service date format: %w", err)
	}
	e.serviceDate = serviceDate

	e.err = cmd.RunExtractAudioWithDependencies(
		context.Background(),
		e.extractor,
		e.fileChecker,
		e.outputDir,
		e.bitrate,
		e.sourcePath,
		e.serviceDate,
		e.output,
	)
	return nil
}

func theAudioOutputFileShouldBe(expected string) error {
	e := getExtractContext()
	if e.resultPath != expected {
		return fmt.Errorf("expected output path %q, got %q", expected, e.resultPath)
	}
	return nil
}

func ffmpegShouldHaveBeenCalledWithAudioArguments(table *godog.Table) error {
	e := getExtractContext()
	if len(e.extractor.calls) == 0 {
		return fmt.Errorf("ffmpeg was not called")
	}

	call := e.extractor.calls[0]

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

func iShouldReceiveAnErrorAboutMissingSourceVideo() error {
	e := getExtractContext()
	if e.err == nil {
		return fmt.Errorf("expected an error but got none")
	}
	if !strings.Contains(e.err.Error(), "does not exist") {
		return fmt.Errorf("expected error about missing source video, got: %v", e.err)
	}
	return nil
}
