// Package setuptest tiene un gestor de Ollama falso para probar la CLI sin instalar ni descargar nada.
package setuptest

import (
	"context"
	"sort"

	"github.com/olipite/vera/internal/setup"
)

// FakeManager simula Ollama. Los campos describen el estado y apuntan lo que se ha pedido.
type FakeManager struct {
	Local     bool             // Ollama está (o estaría) en este equipo
	Installed bool             // Ollama instalado
	Up        bool             // servidor respondiendo
	Models    map[string]int64 // modelos descargados → tamaño
	NotChat   map[string]bool  // modelos que no sirven para conversar
	Remote    map[string]int64 // modelos que existen en el registro → tamaño
	Auto      setup.Autostart

	InstallErr, StartErr, PullErr error

	InstallCalled bool
	StartCalled   bool
	Pulled        []string
	Deleted       []string
	AutostartSet  []bool
}

// New crea un Ollama falso local, instalado y funcionando, sin modelos.
func New() *FakeManager {
	return &FakeManager{Local: true, Installed: true, Up: true, Models: map[string]int64{}, NotChat: map[string]bool{}, Remote: map[string]int64{}}
}

func (f *FakeManager) IsLocal() bool                                  { return f.Local }
func (f *FakeManager) IsInstalled() bool                              { return f.Installed }
func (f *FakeManager) Running(context.Context) bool                   { return f.Up }
func (f *FakeManager) Host() string                                   { return "http://127.0.0.1:11434" }
func (f *FakeManager) AutostartState(context.Context) setup.Autostart { return f.Auto }

func (f *FakeManager) Install(_ context.Context, p func(setup.Progress)) (string, error) {
	f.InstallCalled = true
	if f.InstallErr != nil {
		return "", f.InstallErr
	}
	if p != nil {
		p(setup.Progress{Status: setup.StatusDownloading, Done: 50, Total: 100})
		p(setup.Progress{Status: setup.StatusDownloading, Done: 100, Total: 100})
		p(setup.Progress{Status: setup.StatusExtracting})
	}
	f.Installed = true
	return "/fake/ollama", nil
}

func (f *FakeManager) Start(context.Context) error {
	f.StartCalled = true
	if !f.Installed {
		return setup.ErrNotInstalled
	}
	if f.StartErr != nil {
		return f.StartErr
	}
	f.Up = true
	return nil
}

func (f *FakeManager) EnsureRunning(ctx context.Context) error {
	if f.Up {
		return nil
	}
	if !f.Installed {
		return setup.ErrNotInstalled
	}
	return f.Start(ctx)
}

func (f *FakeManager) find(name string) (string, bool) {
	for n := range f.Models {
		if setup.SameModel(n, name) {
			return n, true
		}
	}
	return "", false
}

func (f *FakeManager) ModelInfo(_ context.Context, name string) (setup.ModelInfo, error) {
	n, ok := f.find(name)
	if !ok {
		return setup.ModelInfo{}, nil
	}
	caps := []string{"completion"}
	if f.NotChat[n] {
		caps = []string{"embedding"}
	}
	return setup.ModelInfo{Exists: true, Capabilities: caps}, nil
}

func (f *FakeManager) RemoteSize(_ context.Context, name string) (int64, error) {
	if s, ok := f.Remote[name]; ok {
		return s, nil
	}
	return 0, setup.ErrModelNotFound
}

func (f *FakeManager) Pull(_ context.Context, name string, p func(setup.Progress)) error {
	f.Pulled = append(f.Pulled, name)
	if f.PullErr != nil {
		return f.PullErr
	}
	size, ok := f.Remote[name]
	if !ok {
		return setup.ErrModelNotFound
	}
	if p != nil {
		p(setup.Progress{Status: setup.StatusDownloading, Done: size / 2, Total: size})
		p(setup.Progress{Status: setup.StatusVerifying, Done: size, Total: size})
		p(setup.Progress{Status: setup.StatusSuccess, Done: size, Total: size})
	}
	f.Models[name] = size
	return nil
}

func (f *FakeManager) List(context.Context) ([]setup.Model, error) {
	var out []setup.Model
	for n, s := range f.Models {
		out = append(out, setup.Model{Name: n, Size: s})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (f *FakeManager) Delete(_ context.Context, name string) error {
	f.Deleted = append(f.Deleted, name)
	if n, ok := f.find(name); ok {
		delete(f.Models, n)
	}
	return nil
}

func (f *FakeManager) SetAutostart(_ context.Context, on bool) error {
	f.AutostartSet = append(f.AutostartSet, on)
	f.Auto.Enabled = on
	f.Auto.State = map[bool]string{true: "sí", false: "no"}[on]
	return nil
}
