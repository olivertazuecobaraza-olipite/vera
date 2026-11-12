package setup

import (
	"archive/tar"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/klauspost/compress/zstd"
)

// download guarda rawURL en un archivo temporal informando del progreso.
// Si falla o se cancela (Ctrl+C), borra el archivo a medias.
func (m *Manager) download(ctx context.Context, rawURL string, p func(Progress)) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := m.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", fmt.Errorf("no hay conexión con %s (%v)", rawURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%s respondió %s", rawURL, resp.Status)
	}
	u, _ := url.Parse(rawURL)
	f, err := os.CreateTemp("", "vera-*-"+filepath.Base(u.Path))
	if err != nil {
		return "", err
	}
	w := &progressWriter{total: resp.ContentLength, p: p}
	_, err = io.Copy(io.MultiWriter(f, w), resp.Body)
	f.Close()
	if err == nil && resp.ContentLength > 0 && w.done != resp.ContentLength {
		err = io.ErrUnexpectedEOF
	}
	if err != nil {
		os.Remove(f.Name())
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", fmt.Errorf("la descarga se cortó (%v). Revisa la conexión y repite", err)
	}
	return f.Name(), nil
}

type progressWriter struct {
	done, total int64
	p           func(Progress)
}

func (w *progressWriter) Write(b []byte) (int, error) {
	w.done += int64(len(b))
	w.p(Progress{Status: StatusDownloading, Done: w.done, Total: w.total})
	return len(b), nil
}

// extractTarZst descomprime un .tar.zst en dest (sin depender del programa zstd).
// Rechaza rutas que intenten salir de dest («../»).
func extractTarZst(file, dest string) error {
	f, err := os.Open(file)
	if err != nil {
		return err
	}
	defer f.Close()
	zr, err := zstd.NewReader(f)
	if err != nil {
		return err
	}
	defer zr.Close()
	tr := tar.NewReader(zr)
	root := filepath.Clean(dest) + string(os.PathSeparator)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		target := filepath.Join(dest, h.Name)
		if !strings.HasPrefix(target, root) {
			return fmt.Errorf("ruta no válida en el archivo: %s", h.Name)
		}
		switch h.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeSymlink:
			os.MkdirAll(filepath.Dir(target), 0o755)
			os.Remove(target)
			if err := os.Symlink(h.Linkname, target); err != nil {
				return err
			}
		case tar.TypeReg:
			os.MkdirAll(filepath.Dir(target), 0o755)
			os.Remove(target) // por si es un ejecutable en uso
			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(h.Mode)&0o777|0o600)
			if err != nil {
				return err
			}
			_, err = io.Copy(out, tr)
			out.Close()
			if err != nil {
				return err
			}
		}
	}
}
