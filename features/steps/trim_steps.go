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
	calls       []trimCall
	shouldFail  bool
	failError   error
	fileChecker *mockFileChecker // Reference to mark output files as existing
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
	// Mark the output file as existing so subsequent operations can find it
	if m.fileChecker != nil {
		m.fileChecker.existingFiles[outputPath] = true
	}
	return nil
}

// mockFileChecker simulates file existence
type mockFileChecker struct {
	existingFiles map[string]bool
}

func (m *mockFileChecker) Exists(path string) bool {
	return m.existingFiles[path]
}

// mockTrimExtractor records calls to Extract for verification in trim+audio tests
type mockTrimExtractor struct {
	calls      []trimExtractCall
	shouldFail bool
	failError  error
}

type trimExtractCall struct {
	req        *video.AudioExtractionRequest
	outputPath string
	args       []string
}

func (m *mockTrimExtractor) Extract(ctx context.Context, req *video.AudioExtractionRequest, outputPath string) error {
	if m.shouldFail {
		return m.failError
	}
	m.calls = append(m.calls, trimExtractCall{
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

// trimContext holds test state for trim scenarios
type trimContext struct {
	sourcePath      string
	outputDir       string
	startTime       string
	endTime         string
	trimmer         *mockTrimmer
	fileChecker     *mockFileChecker
	extractor       *mockTrimExtractor
	audioOutputDir  string
	audioBitrate    string
	output          *bytes.Buffer
	err             error
	resultPath      string
	audioResultPath string
}

// SharedTrimContext is reset before each scenario via Before hook
var SharedTrimContext *trimContext

func getTrimContext() *trimContext {
	return SharedTrimContext
}

func InitializeTrimScenario(ctx *godog.ScenarioContext) {
	ctx.Before(func(c context.Context, sc *godog.Scenario) (context.Context, error) {
		fileChecker := &mockFileChecker{
			existingFiles: make(map[string]bool),
		}
		SharedTrimContext = &trimContext{
			trimmer:     &mockTrimmer{fileChecker: fileChecker},
			fileChecker: fileChecker,
			extractor:   &mockTrimExtractor{},
			output:      &bytes.Buffer{},
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

	// Steps for trim with audio extraction
	ctx.Step(`^the trim audio output directory is "([^"]*)"$`, theTrimAudioOutputDirectoryIs)
	ctx.Step(`^the trim audio bitrate is "([^"]*)"$`, theAudioBitrateIs)
	ctx.Step(`^I trim the video from "([^"]*)" to "([^"]*)" with audio extraction$`, iTrimTheVideoFromToWithAudioExtraction)
	ctx.Step(`^the trim audio output file should be "([^"]*)"$`, theTrimAudioOutputFileShouldBe)
	ctx.Step(`^the trim audio extraction should have used arguments:$`, ffmpegShouldHaveBeenCalledWithAudioArgumentsTrim)
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
		nil, // no audio extractor
		"",  // no audio output dir
		"",  // no audio bitrate
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
		nil, // no audio extractor
		"",  // no audio output dir
		"",  // no audio bitrate
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
		nil, // no audio extractor
		"",  // no audio output dir
		"",  // no audio bitrate
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

// Steps for trim with audio extraction

func theTrimAudioOutputDirectoryIs(dir string) error {
	t := getTrimContext()
	t.audioOutputDir = dir
	return nil
}

func theAudioBitrateIs(bitrate string) error {
	t := getTrimContext()
	t.audioBitrate = bitrate
	return nil
}

func iTrimTheVideoFromToWithAudioExtraction(start, end string) error {
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
		t.extractor,
		t.audioOutputDir,
		t.audioBitrate,
		t.output,
	)

	if t.err != nil {
		return fmt.Errorf("unexpected error: %v", t.err)
	}

	// Capture result paths from mock calls
	if len(t.trimmer.calls) > 0 {
		t.resultPath = t.trimmer.calls[0].outputPath
		// Mark the trimmed file as existing for the extractor
		t.fileChecker.existingFiles[t.resultPath] = true
	}
	if len(t.extractor.calls) > 0 {
		t.audioResultPath = t.extractor.calls[0].outputPath
	}
	return nil
}

func theTrimAudioOutputFileShouldBe(expected string) error {
	t := getTrimContext()
	if t.audioResultPath != expected {
		return fmt.Errorf("expected audio output path %q, got %q", expected, t.audioResultPath)
	}
	return nil
}

func ffmpegShouldHaveBeenCalledWithAudioArgumentsTrim(table *godog.Table) error {
	t := getTrimContext()
	if len(t.extractor.calls) == 0 {
		return fmt.Errorf("ffmpeg audio extraction was not called")
	}

	call := t.extractor.calls[0]

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
			return fmt.Errorf("expected argument %q not found in ffmpeg audio call: %v", expectedArg, call.args)
		}
	}
	return nil
}
