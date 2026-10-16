package cmd

import (
	"context"
	"fmt"
	"image"
	_ "image/png" // for image.DecodeConfig
	"io"
	"os"
	"path/filepath"

	"nac-service-media/domain/video"
	"nac-service-media/infrastructure/config"
	infradetection "nac-service-media/infrastructure/detection"

	"github.com/spf13/cobra"
)

var (
	templatesInput   string
	templatesLitAt   string
	templatesUnlitAt string
	templatesName    string
	templatesRegion  string
	templatesForce   bool
	templatesAt      string
	templatesOut     string
)

var templatesCmd = &cobra.Command{
	Use:   "templates",
	Short: "Teach the cross detector a camera shot",
	Long: `The start of the service is found by matching video frames against
pictures of the cross, lit and unlit, one pair per camera shot, in the
templates folder (detection.templates_dir, default config/detection_templates).
When the camera's shot changes, add the new shot with 'templates add'.`,
}

var templatesAddCmd = &cobra.Command{
	Use:          "add",
	Short:        "Learn a new cross shot from a video, a lit time and an unlit time",
	SilenceUsage: true,
	Long: `Learn a new cross shot from a video: give a time when the cross is lit
and a time when it is not (a second or two apart is best, so nothing else
moves). The cross's box is found as the part of the picture that brightens
most between the two; give --region x,y,w,h to choose it yourself. Both frames
are cropped to the box and saved as <name>_lit.png and <name>_unlit.png in the
templates folder. An existing name is kept unless --force is given.

Example:
  nac-service-media templates add --input "2026-10-11 10-17-09.mp4" \
    --unlit-at 00:14:06 --lit-at 00:14:07 --name room`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := GetConfig()
		req := infradetection.AddTemplateRequest{
			VideoPath:    resolveVideoInput(templatesInput, cfg),
			LitAt:        templatesLitAt,
			UnlitAt:      templatesUnlitAt,
			Name:         templatesName,
			TemplatesDir: templatesDirFrom(cfg),
			Force:        templatesForce,
		}
		if templatesRegion != "" {
			region, err := infradetection.ParseRegion(templatesRegion)
			if err != nil {
				return fmt.Errorf("--region: %w", err)
			}
			req.Region = region
		}
		return templatesAdd(cmd.Context(), infradetection.FFmpegFrames{}, req, os.Stdout)
	},
}

var templatesFrameCmd = &cobra.Command{
	Use:          "frame",
	Short:        "Write one full frame of a video to a PNG, to look at a shot",
	SilenceUsage: true,
	Long: `Write the full frame of a video at one time to a PNG, so you can look at
the shot, find the cross and check a time.

Example:
  nac-service-media templates frame --input "2026-10-11 10-17-09.mp4" --at 00:14:07 --out cross.png`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return templatesFrame(cmd.Context(), infradetection.FFmpegFrames{}, resolveVideoInput(templatesInput, GetConfig()), templatesAt, templatesOut, os.Stdout)
	},
}

func init() {
	rootCmd.AddCommand(templatesCmd)
	templatesCmd.AddCommand(templatesAddCmd, templatesFrameCmd)

	templatesAddCmd.Flags().StringVar(&templatesInput, "input", "", "Video file (a bare name is looked for in the source directory)")
	templatesAddCmd.Flags().StringVar(&templatesLitAt, "lit-at", "", "A time when the cross is lit, HH:MM:SS")
	templatesAddCmd.Flags().StringVar(&templatesUnlitAt, "unlit-at", "", "A time when the cross is not lit, HH:MM:SS")
	templatesAddCmd.Flags().StringVar(&templatesName, "name", "", "Name of the shot, e.g. room")
	templatesAddCmd.Flags().StringVar(&templatesRegion, "region", "", "The cross's box as x,y,w,h in pixels (found by itself when omitted)")
	templatesAddCmd.Flags().BoolVar(&templatesForce, "force", false, "Replace a shot of the same name")
	for _, name := range []string{"input", "lit-at", "unlit-at", "name"} {
		templatesAddCmd.MarkFlagRequired(name)
	}

	templatesFrameCmd.Flags().StringVar(&templatesInput, "input", "", "Video file (a bare name is looked for in the source directory)")
	templatesFrameCmd.Flags().StringVar(&templatesAt, "at", "", "Time of the frame, HH:MM:SS")
	templatesFrameCmd.Flags().StringVar(&templatesOut, "out", "", "PNG file to write")
	for _, name := range []string{"input", "at", "out"} {
		templatesFrameCmd.MarkFlagRequired(name)
	}
}

// templatesAdd learns a shot and prints the box chosen and the files written.
func templatesAdd(ctx context.Context, frames infradetection.FrameExtractor, req infradetection.AddTemplateRequest, out io.Writer) error {
	res, err := infradetection.AddTemplate(ctx, frames, req)
	if err != nil {
		return err
	}
	how := "found from the brightness rise between the two frames"
	if res.FromRegion {
		how = "from --region"
	}
	fmt.Fprintf(out, "Cross box: %s in a %dx%d frame (%s)\n", infradetection.FormatRegion(res.Box), res.FrameSize.X, res.FrameSize.Y, how)
	fmt.Fprintf(out, "Wrote %s\n", res.LitPath)
	fmt.Fprintf(out, "Wrote %s\n", res.UnlitPath)
	fmt.Fprintf(out, "Look at the two pictures: each should show the cross with a little room around it. If not, run again with --region x,y,w,h and --force.\n")
	return nil
}

// templatesFrame writes one full frame to a PNG.
func templatesFrame(ctx context.Context, frames infradetection.FrameExtractor, videoPath, at, outPath string, out io.Writer) error {
	if _, err := video.ParseTimestamp(at); err != nil {
		return fmt.Errorf("--at: %w", err)
	}
	if dir := filepath.Dir(outPath); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("failed to create %s: %w", dir, err)
		}
	}
	if err := frames.ExtractFrame(ctx, videoPath, at, outPath); err != nil {
		return err
	}
	size := ""
	if file, err := os.Open(outPath); err == nil {
		if cfg, _, err := image.DecodeConfig(file); err == nil {
			size = fmt.Sprintf(" (%dx%d)", cfg.Width, cfg.Height)
		}
		file.Close()
	}
	fmt.Fprintf(out, "Wrote %s%s, the frame at %s of %s\n", outPath, size, at, videoPath)
	return nil
}

// resolveVideoInput finds a video given on the command line: as given when it
// exists or is absolute, else in the configured source directory.
func resolveVideoInput(input string, cfg *config.Config) string {
	if filepath.IsAbs(input) || cfg == nil || cfg.Paths.SourceDirectory == "" {
		return input
	}
	if _, err := os.Stat(input); err == nil {
		return input
	}
	return filepath.Join(cfg.Paths.SourceDirectory, input)
}

// templatesDirFrom is the configured templates folder, or the default.
func templatesDirFrom(cfg *config.Config) string {
	if cfg != nil && cfg.Detection.TemplatesDir != "" {
		return cfg.Detection.TemplatesDir
	}
	return infradetection.DefaultTemplatesDir
}
