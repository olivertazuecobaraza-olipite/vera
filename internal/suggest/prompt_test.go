package suggest_test

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/olipite/vera/internal/suggest"
	"github.com/olipite/vera/internal/sysinfo"
)

var update = flag.Bool("update", false, "regenera los golden files")

// Si el prompt cambia sin querer, estos tests fallan y enseñan la diferencia.
// Para cambiarlo a propósito: go test ./internal/suggest -update  (y revisar con git diff).
func TestBuildPromptGolden(t *testing.T) {
	tests := map[string]sysinfo.Info{
		"prompt_arch_zsh.golden":           archZsh(),
		"prompt_macos_zsh.golden":          {OS: "darwin", OSName: "macOS", Arch: "arm64", Shell: "zsh", ShellKind: "posix", PkgManager: "brew", Cwd: "/Users/ana"},
		"prompt_windows_powershell.golden": {OS: "windows", OSName: "Windows", Arch: "amd64", Shell: "PowerShell 7", ShellKind: "powershell", PkgManager: "winget", Cwd: `C:\Users\ana`},
		"prompt_windows_cmd.golden":        {OS: "windows", OSName: "Windows", Arch: "amd64", Shell: "cmd.exe", ShellKind: "cmd", Cwd: `C:\Users\ana`},
	}
	for file, info := range tests {
		t.Run(file, func(t *testing.T) {
			got := suggest.BuildPrompt(info)
			golden := filepath.Join("testdata", file)
			if *update {
				os.MkdirAll("testdata", 0o755)
				if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatalf("no existe el golden; ejecuta con -update: %v", err)
			}
			if got != string(want) {
				t.Errorf("el prompt ha cambiado.\n--- got ---\n%s\n--- want ---\n%s", got, want)
			}
		})
	}
}
