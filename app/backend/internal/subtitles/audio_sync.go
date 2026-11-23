package subtitles

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/importer"
	"github.com/HolyArrs/holyarrs/app/backend/internal/subtitles/subtext"
)

// One alignment at a time keeps searches from decoding multiple videos at once.
var audioSyncSlot = make(chan struct{}, 1)

// alignSubtitleToAudio aligns downloaded SRT cues to detected speech in the
// video. Keep the provider's timings if the tool is unavailable, the input
// is outside the library, or the alignment is unsuccessful.
func alignSubtitleToAudio(ctx context.Context, fc *fileCtx, text, format string) (string, bool) {
	bin := os.Getenv("HOLYARRS_SUBTITLE_SYNC_BIN")
	if bin == "" || format != subtext.SRT {
		return text, false
	}
	video := fc.video()
	if !filepath.IsAbs(video) || !importer.InsideReal(fc.lib, video) {
		return text, false
	}
	if fi, err := os.Stat(video); err != nil || !fi.Mode().IsRegular() {
		return text, false
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	select {
	case audioSyncSlot <- struct{}{}:
		defer func() { <-audioSyncSlot }()
	case <-ctx.Done():
		return text, false
	}
	dir, err := os.MkdirTemp("", "holyarrs-subtitle-sync-")
	if err != nil {
		return text, false
	}
	defer os.RemoveAll(dir)
	input, output := filepath.Join(dir, "source.srt"), filepath.Join(dir, "aligned.srt")
	if err := os.WriteFile(input, []byte(text), 0o600); err != nil {
		return text, false
	}
	cmd := exec.CommandContext(ctx, bin, video, "-i", input, "-o", output,
		"--vad", "webrtc", "--max-offset-seconds", "30", "--skip-sync-on-low-quality", "--encoding", "utf-8", "--output-encoding", "utf-8")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "OPENBLAS_NUM_THREADS=1", "OMP_NUM_THREADS=1")
	cmd.WaitDelay = 5 * time.Second
	if err := cmd.Run(); err != nil {
		return text, false
	}
	f, err := os.Open(output)
	if err != nil {
		return text, false
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, subtext.MaxText+1))
	if err != nil || len(data) > subtext.MaxText {
		return text, false
	}
	clean, err := subtext.Clean(data, format)
	if err != nil || clean == text {
		return text, false
	}
	return clean, true
}
