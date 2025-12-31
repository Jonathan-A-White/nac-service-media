package video

import (
	"fmt"
	"path/filepath"
	"regexp"
	"time"
)

// TrimRequest represents a request to trim a video
type TrimRequest struct {
	SourcePath  string
	Start       Timestamp
	End         Timestamp
	ServiceDate time.Time
}

// sourceFilenameRegex matches OBS output format: YYYY-MM-DD HH-MM-SS.mp4
var sourceFilenameRegex = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2}) \d{2}-\d{2}-\d{2}\.mp4$`)

// NewTrimRequest creates a new TrimRequest, parsing the service date from the source filename
func NewTrimRequest(sourcePath string, start, end Timestamp) (*TrimRequest, error) {
	filename := filepath.Base(sourcePath)

	matches := sourceFilenameRegex.FindStringSubmatch(filename)
	if matches == nil {
		return nil, fmt.Errorf("source filename %q does not match expected format 'YYYY-MM-DD HH-MM-SS.mp4'", filename)
	}

	dateStr := matches[1]
	serviceDate, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return nil, fmt.Errorf("invalid date in filename: %w", err)
	}

	req := &TrimRequest{
		SourcePath:  sourcePath,
		Start:       start,
		End:         end,
		ServiceDate: serviceDate,
	}

	if err := req.Validate(); err != nil {
		return nil, err
	}

	return req, nil
}

// Validate checks that the trim request is valid
func (r *TrimRequest) Validate() error {
	if r.SourcePath == "" {
		return fmt.Errorf("source path is required")
	}

	if r.End.Before(r.Start) || r.End.TotalSeconds() == r.Start.TotalSeconds() {
		return fmt.Errorf("end time %s must be after start time %s", r.End, r.Start)
	}

	return nil
}

// OutputFilename returns the output filename in YYYY-MM-DD.mp4 format
func (r *TrimRequest) OutputFilename() string {
	return r.ServiceDate.Format("2006-01-02") + ".mp4"
}

// OutputPath returns the full output path given an output directory
func (r *TrimRequest) OutputPath(outputDir string) string {
	return filepath.Join(outputDir, r.OutputFilename())
}
