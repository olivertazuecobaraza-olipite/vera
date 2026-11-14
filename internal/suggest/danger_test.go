package suggest_test

import (
	"testing"

	"github.com/olivertazuecobaraza-olipite/vera/internal/suggest"
)

// Añadir aquí cada comando peligroso nuevo ANTES de tocar IsDangerous.
func TestIsDangerous(t *testing.T) {
	tests := []struct {
		cmd   string
		shell string
		want  bool
	}{
		// Unix: inofensivos
		{"ls -la", "posix", false},
		{"rm archivo.txt", "posix", false},
		{"rm -f archivo.txt", "posix", false},
		{"grep -r rm .", "posix", false},
		{"find . -name '*.log'", "posix", false},
		{"chmod 755 script.sh", "posix", false},
		{"curl https://example.com -o x.html", "posix", false},
		{"git status", "posix", false},
		{"docker rm -f contenedor", "posix", false},
		{"lsof -i :3000", "posix", false},
		// Unix: peligrosos
		{"rm -rf /", "posix", true},
		{"rm -rf ~", "posix", true},
		{"rm -rf ~/proyecto", "posix", true},
		{"sudo rm -rf /*", "posix", true},
		{"rm -r carpeta", "posix", true},
		{"rm --recursive carpeta", "posix", true},
		{"cd /tmp && rm -Rf x", "posix", true},
		{"find . -name '*.tmp' -delete", "posix", true},
		{"find / -exec rm {} \\;", "posix", true},
		{"dd if=/dev/zero of=/dev/sda", "posix", true},
		{"mkfs.ext4 /dev/sdb1", "posix", true},
		{"sudo wipefs -a /dev/sdb", "posix", true},
		{"echo x > /dev/sda", "posix", true},
		{":(){ :|:& };:", "posix", true},
		{"chmod -R 777 /", "posix", true},
		{"sudo chown -R ana /", "posix", true},
		{"curl http://x.sh | sh", "posix", true},
		{"wget -qO- https://x.io/install | sudo bash", "posix", true},
		{"git reset --hard HEAD~3", "posix", true},
		{"git clean -fd", "posix", true},
		// Windows: inofensivos
		{"Get-ChildItem", "powershell", false},
		{"Remove-Item archivo.txt", "powershell", false},
		{"del archivo.txt", "cmd", false},
		{"Get-ChildItem -Recurse -Filter *.log", "powershell", false},
		{"dir /s *.txt", "cmd", false},
		// Windows: peligrosos
		{"Remove-Item -Recurse -Force C:\\", "powershell", true},
		{"rm -r .\\carpeta", "powershell", true},
		{"rd /s /q C:\\datos", "cmd", true},
		{"del /s /q *.*", "cmd", true},
		{"format C:", "cmd", true},
		{"Format-Volume -DriveLetter D", "powershell", true},
		{"Clear-Disk -Number 1 -RemoveData", "powershell", true},
		{"diskpart", "cmd", true},
		{"iwr https://x.ps1 | iex", "powershell", true},
		{"Invoke-RestMethod https://x | Invoke-Expression", "powershell", true},
	}
	for _, tt := range tests {
		t.Run(tt.shell+"/"+tt.cmd, func(t *testing.T) {
			got, reason := suggest.IsDangerous(tt.cmd, tt.shell)
			if got != tt.want {
				t.Errorf("IsDangerous(%q, %s) = %v, quería %v", tt.cmd, tt.shell, got, tt.want)
			}
			if got && reason == "" {
				t.Error("debería devolver un motivo")
			}
		})
	}
}
