// Package sysinfo detecta el sistema donde corre vera: SO, distribución, shell y gestor de paquetes.
// Es lo que más mejora las sugerencias: en Arch con zsh no hay que proponer «apt install».
package sysinfo

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// Info describe el sistema.
type Info struct {
	OS         string   // runtime.GOOS: "darwin", "linux", "windows"
	OSName     string   // nombre legible: "macOS", "Ubuntu 24.04 LTS", "Windows"
	Distro     string   // Linux: ID de /etc/os-release ("arch", "ubuntu"…)
	Arch       string   // runtime.GOARCH
	Shell      string   // nombre legible: "zsh", "PowerShell 7", "cmd.exe"
	ShellPath  string   // ejecutable de la shell
	ShellArgs  []string // argumentos antes del comando: ["-c"], ["-NoProfile", "-Command"], ["/C"]
	ShellKind  string   // "posix", "powershell" o "cmd"
	PkgManager string   // "pacman", "apt", "brew", "winget"… o "" si no se sabe
	Cwd        string
}

// Detect detecta el sistema real.
func Detect() Info {
	cwd, _ := os.Getwd()
	return detect(env{
		goos: runtime.GOOS, goarch: runtime.GOARCH, cwd: cwd,
		getenv: os.Getenv, readFile: os.ReadFile, lookPath: exec.LookPath,
	})
}

// env son las dependencias de detect, inyectables en los tests.
type env struct {
	goos, goarch, cwd string
	getenv            func(string) string
	readFile          func(string) ([]byte, error)
	lookPath          func(string) (string, error)
}

func detect(e env) Info {
	i := Info{OS: e.goos, Arch: e.goarch, Cwd: e.cwd}
	switch e.goos {
	case "darwin":
		i.OSName = "macOS"
		if has(e, "brew") {
			i.PkgManager = "brew"
		}
	case "windows":
		i.OSName = "Windows"
		switch {
		case has(e, "winget"):
			i.PkgManager = "winget"
		case has(e, "choco"):
			i.PkgManager = "choco"
		case has(e, "scoop"):
			i.PkgManager = "scoop"
		}
	case "linux":
		rel := osRelease(e)
		i.OSName, i.Distro = rel["PRETTY_NAME"], rel["ID"]
		if i.OSName == "" {
			i.OSName = "Linux"
		}
		i.PkgManager = linuxPkgManager(e, rel["ID"], rel["ID_LIKE"])
	default:
		i.OSName = e.goos
	}
	i.Shell, i.ShellPath, i.ShellArgs, i.ShellKind = shell(e)
	return i
}

func has(e env, bin string) bool {
	_, err := e.lookPath(bin)
	return err == nil
}

func osRelease(e env) map[string]string {
	m := map[string]string{}
	b, err := e.readFile("/etc/os-release")
	if err != nil {
		return m
	}
	for _, l := range strings.Split(string(b), "\n") {
		if k, v, ok := strings.Cut(strings.TrimSpace(l), "="); ok {
			m[k] = strings.Trim(v, `"'`)
		}
	}
	return m
}

var pkgByDistro = []struct {
	ids []string
	pm  string
}{
	{[]string{"arch", "manjaro", "endeavouros", "garuda"}, "pacman"},
	{[]string{"debian", "ubuntu", "linuxmint", "pop", "elementary", "kali", "raspbian"}, "apt"},
	{[]string{"fedora", "rhel", "centos", "rocky", "almalinux", "amzn"}, "dnf"},
	{[]string{"opensuse", "opensuse-leap", "opensuse-tumbleweed", "sles", "suse"}, "zypper"},
	{[]string{"alpine"}, "apk"},
	{[]string{"void"}, "xbps-install"},
	{[]string{"gentoo"}, "emerge"},
	{[]string{"nixos"}, "nix"},
}

// linuxPkgManager decide por la distribución (ID y luego ID_LIKE) y, si no, por lo que haya instalado.
func linuxPkgManager(e env, id, idLike string) string {
	for _, cand := range append([]string{id}, strings.Fields(idLike)...) {
		for _, d := range pkgByDistro {
			for _, x := range d.ids {
				if cand == x {
					return d.pm
				}
			}
		}
	}
	for _, pm := range []string{"apt", "dnf", "pacman", "zypper", "apk"} {
		if has(e, pm) {
			return pm
		}
	}
	return ""
}

// shell decide con qué shell se ejecutan los comandos. VERA_SHELL manda; en Windows, "cmd" elige cmd.exe.
func shell(e env) (name, path string, args []string, kind string) {
	if e.goos == "windows" {
		if strings.EqualFold(e.getenv("VERA_SHELL"), "cmd") {
			return "cmd.exe", "cmd.exe", []string{"/C"}, "cmd"
		}
		if p, err := e.lookPath("pwsh"); err == nil {
			return "PowerShell 7", p, []string{"-NoProfile", "-Command"}, "powershell"
		}
		return "Windows PowerShell", "powershell.exe", []string{"-NoProfile", "-Command"}, "powershell"
	}
	sh := e.getenv("VERA_SHELL")
	if sh == "" {
		sh = e.getenv("SHELL")
	}
	if sh == "" {
		sh = "/bin/sh"
	}
	return filepath.Base(sh), sh, []string{"-c"}, "posix"
}
