package detection

import (
	"os/exec"
	"path/filepath"
	"testing"
)

// The amen search window is worked out in scripts/detect_amen.py as a pure
// function; this runs its unittest in the gate. It needs python3 but not
// librosa.
func TestAmenSearchWindow_PythonUnitTests(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 is not installed")
	}
	scripts, err := filepath.Abs(filepath.Join("..", "..", "scripts"))
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(python, "-m", "unittest", "test_detect_amen", "-v")
	cmd.Dir = scripts
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("python unittest failed: %v\n%s", err, out)
	}
}
