package report

import (
	"strings"
	"testing"
)

func TestProjecteActuacioRenderer_SkeletonAndActivities(t *testing.T) {
	out := string(ProjecteActuacioRenderer{}.Render(projData2025(t)))

	mustContain(t, out, "# Projecte d'actuació 2025")
	mustContain(t, out, "_[Introducció")
	mustContain(t, out, "## Objectius")
	mustContain(t, out, "### a.2. Activitats d'informació i promoció")
	mustContain(t, out, "_[Objectius d'aquest apartat]_")
	mustContain(t, out, "## Activitats")
	mustContain(t, out, "### a.6. Despeses de fertilitzants")
	// Activity line = Concept (CP…), no amount.
	mustContain(t, out, "- Adob orgànic (CP25006, CP25007)")
	mustContain(t, out, "- Carretilla transportadora (CP25028)")
	mustContain(t, out, "President")

	// No euro amounts leak into the narrative.
	if strings.Contains(out, "€") {
		t.Errorf("narrative must not contain amounts, got:\n%s", out)
	}
}
