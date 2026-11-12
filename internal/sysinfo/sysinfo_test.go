package sysinfo

import (
	"errors"
	"testing"
)

func fakeEnv(goos string, vars map[string]string, osRelease string, bins ...string) env {
	return env{
		goos: goos, goarch: "amd64", cwd: "/home/ana",
		getenv: func(k string) string { return vars[k] },
		readFile: func(string) ([]byte, error) {
			if osRelease == "" {
				return nil, errors.New("no existe")
			}
			return []byte(osRelease), nil
		},
		lookPath: func(name string) (string, error) {
			for _, b := range bins {
				if b == name {
					return `C:\bin\` + name, nil
				}
			}
			return "", errors.New("no encontrado")
		},
	}
}

func TestDetectLinux(t *testing.T) {
	tests := []struct {
		name, osRelease      string
		bins                 []string
		wantName, wantDistro string
		wantPkg              string
	}{
		{"arch", "NAME=\"Arch Linux\"\nPRETTY_NAME=\"Arch Linux\"\nID=arch\n", nil, "Arch Linux", "arch", "pacman"},
		{"ubuntu", "PRETTY_NAME=\"Ubuntu 24.04.1 LTS\"\nID=ubuntu\nID_LIKE=debian\n", nil, "Ubuntu 24.04.1 LTS", "ubuntu", "apt"},
		{"fedora", "PRETTY_NAME=\"Fedora Linux 41\"\nID=fedora\n", nil, "Fedora Linux 41", "fedora", "dnf"},
		{"derivada por ID_LIKE", "PRETTY_NAME=\"Zorin OS 17\"\nID=zorin\nID_LIKE=\"ubuntu debian\"\n", nil, "Zorin OS 17", "zorin", "apt"},
		{"opensuse", "PRETTY_NAME=\"openSUSE Tumbleweed\"\nID=\"opensuse-tumbleweed\"\nID_LIKE=\"opensuse suse\"\n", nil, "openSUSE Tumbleweed", "opensuse-tumbleweed", "zypper"},
		{"alpine", "PRETTY_NAME=\"Alpine Linux v3.20\"\nID=alpine\n", nil, "Alpine Linux v3.20", "alpine", "apk"},
		{"desconocida con dnf instalado", "ID=rara\n", []string{"dnf"}, "Linux", "rara", "dnf"},
		{"sin os-release", "", nil, "Linux", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detect(fakeEnv("linux", map[string]string{"SHELL": "/usr/bin/zsh"}, tt.osRelease, tt.bins...))
			if got.OSName != tt.wantName || got.Distro != tt.wantDistro || got.PkgManager != tt.wantPkg {
				t.Errorf("got %q / %q / %q", got.OSName, got.Distro, got.PkgManager)
			}
			if got.Shell != "zsh" || got.ShellKind != "posix" || got.ShellPath != "/usr/bin/zsh" {
				t.Errorf("shell = %+v", got)
			}
		})
	}
}

func TestDetectMac(t *testing.T) {
	got := detect(fakeEnv("darwin", map[string]string{"SHELL": "/bin/zsh"}, "", "brew"))
	if got.OSName != "macOS" || got.PkgManager != "brew" || got.Shell != "zsh" {
		t.Errorf("got %+v", got)
	}
	if got := detect(fakeEnv("darwin", nil, "")); got.PkgManager != "" || got.ShellPath != "/bin/sh" {
		t.Errorf("sin brew ni SHELL: %+v", got)
	}
}

func TestVeraShellManda(t *testing.T) {
	got := detect(fakeEnv("linux", map[string]string{"SHELL": "/bin/zsh", "VERA_SHELL": "/bin/bash"}, ""))
	if got.Shell != "bash" {
		t.Errorf("VERA_SHELL debería mandar: %q", got.Shell)
	}
}

func TestDetectWindows(t *testing.T) {
	tests := []struct {
		name      string
		vars      map[string]string
		bins      []string
		wantShell string
		wantKind  string
		wantPkg   string
	}{
		{"PowerShell 7 si está", nil, []string{"pwsh", "winget"}, "PowerShell 7", "powershell", "winget"},
		{"Windows PowerShell si no", nil, nil, "Windows PowerShell", "powershell", ""},
		{"cmd con VERA_SHELL", map[string]string{"VERA_SHELL": "CMD"}, []string{"pwsh", "scoop"}, "cmd.exe", "cmd", "scoop"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detect(fakeEnv("windows", tt.vars, "", tt.bins...))
			if got.OSName != "Windows" || got.Shell != tt.wantShell || got.ShellKind != tt.wantKind || got.PkgManager != tt.wantPkg {
				t.Errorf("got %+v", got)
			}
		})
	}
}
