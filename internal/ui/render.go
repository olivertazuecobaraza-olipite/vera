package ui

// Suggestion pinta un comando propuesto: en cian si es normal, en rojo y con aviso si es peligroso.
//
//	❯ lsof -i :3000
//	  Muestra qué proceso tiene abierto el puerto 3000.
func (t *Terminal) Suggestion(command, explanation string, dangerous bool, reason string) {
	if dangerous {
		t.Println(t.Paint(Red, "❯ "+command))
	} else {
		t.Println(t.Paint(Cyan, "❯"), command)
	}
	if explanation != "" {
		t.Println(t.Paint(Gray, "  "+explanation))
	}
	if dangerous {
		if reason == "" {
			reason = "revisa bien lo que hace antes de ejecutarlo."
		}
		t.Println(t.Paint(Red, "  ⚠ Comando peligroso: "+reason))
	}
}
