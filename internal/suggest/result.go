package suggest

import (
	"encoding/json"
	"errors"
	"regexp"
	"strings"
)

// Suggestion es un comando propuesto.
type Suggestion struct {
	Command     string `json:"command"`
	Explanation string `json:"explanation"`
	Dangerous   bool   `json:"dangerous"`
	Reason      string `json:"-"` // motivo del peligro (lo rellena vera, no el modelo)
}

// ErrEmpty: el modelo no devolvió ningún comando.
var ErrEmpty = errors.New("el modelo no devolvió ningún comando")

var thinkRe = regexp.MustCompile(`(?s)<think>.*?</think>`)

// parse interpreta la respuesta del modelo: JSON con esquema y, si un modelo no lo respeta,
// se queda con la primera línea útil del texto (plan B).
func parse(raw string) (Suggestion, error) {
	raw = strings.TrimSpace(thinkRe.ReplaceAllString(raw, ""))
	var s Suggestion
	if err := json.Unmarshal([]byte(raw), &s); err == nil {
		s.Command = cleanCommand(s.Command)
		s.Explanation = strings.TrimSpace(s.Explanation)
	} else {
		s = Suggestion{Command: cleanCommand(raw)}
	}
	if s.Command == "" {
		return Suggestion{}, ErrEmpty
	}
	return s, nil
}

var fenceRe = regexp.MustCompile("^```[a-zA-Z]*$")

// cleanCommand se queda con la primera línea útil: sin bloques de código, comillas invertidas ni prompts («$ »).
func cleanCommand(s string) string {
	for _, l := range strings.Split(s, "\n") {
		l = strings.TrimSpace(l)
		if l == "" || fenceRe.MatchString(l) {
			continue
		}
		l = strings.Trim(l, "`")
		for _, p := range []string{"$ ", "PS> ", "> "} {
			l = strings.TrimPrefix(l, p)
		}
		return strings.TrimSpace(l)
	}
	return ""
}
