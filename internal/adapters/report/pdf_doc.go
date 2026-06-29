package report

import (
	"fmt"
	"os"

	"github.com/johnfercher/maroto/pkg/color"
	"github.com/johnfercher/maroto/pkg/consts"
	"github.com/johnfercher/maroto/pkg/pdf"
	"github.com/johnfercher/maroto/pkg/props"
)

var headingColor = color.Color{Red: 0, Green: 51, Blue: 51}
var redColor = color.Color{Red: 200, Green: 0, Blue: 0}

// renderDocument builds an A4 portrait PDF from the blocks and returns its bytes.
func renderDocument(title, footer, businessName, logoPath string, blocks []Block) ([]byte, error) {
	m := pdf.NewMaroto(consts.Portrait, consts.A4)
	m.SetPageMargins(15, 10, 15)

	registerHeader(m, businessName, logoPath)
	registerFooter(m, footer)
	docTitle(m, title)

	for _, blk := range blocks {
		switch v := blk.(type) {
		case SectionTitle:
			sectionTitle(m, v.Text)
		case PageBreak:
			m.AddPage()
		case Table:
			renderTable(m, v)
		}
	}

	out, err := m.Output()
	if err != nil {
		return nil, fmt.Errorf("rendering PDF: %w", err)
	}
	return out.Bytes(), nil
}

func registerHeader(m pdf.Maroto, businessName, logoPath string) {
	hasLogo := false
	if logoPath != "" {
		if _, err := os.Stat(logoPath); err == nil {
			hasLogo = true
		}
	}
	m.RegisterHeader(func() {
		m.Row(20, func() {
			if hasLogo {
				m.Col(3, func() { _ = m.FileImage(logoPath, props.Rect{Center: true, Percent: 80}) })
				m.ColSpace(5)
				m.Col(4, func() {
					m.Text(businessName, props.Text{Style: consts.BoldItalic, Size: 10, Align: consts.Left})
				})
			} else {
				m.Col(12, func() {
					m.Text(businessName, props.Text{Style: consts.BoldItalic, Size: 10, Align: consts.Left})
				})
			}
		})
	})
}

func registerFooter(m pdf.Maroto, footer string) {
	if footer == "" {
		return
	}
	m.RegisterFooter(func() {
		m.Row(4, func() {
			m.Col(12, func() {
				m.Text(footer, props.Text{Top: 1, Style: consts.Italic, Size: 8, Align: consts.Right})
			})
		})
	})
}

func docTitle(m pdf.Maroto, title string) {
	m.Row(20, func() {
		m.Col(12, func() {
			m.Text(title, props.Text{Top: 4, Style: consts.Bold, Align: consts.Center, Color: headingColor, Size: 18})
		})
	})
}

func sectionTitle(m pdf.Maroto, text string) {
	m.Row(14, func() {
		m.Col(12, func() {
			m.Text(text, props.Text{Top: 6, Style: consts.Bold, Align: consts.Left, Color: headingColor, Size: 13})
		})
	})
}

func renderTable(m pdf.Maroto, t Table) {
	if t.Title != "" {
		m.Row(12, func() {
			m.Col(12, func() {
				m.Text(t.Title, props.Text{Top: 4, Style: consts.Bold, Align: consts.Left, Color: headingColor, Size: 11})
			})
		})
	}
	// header
	if hasNonEmpty(t.Headers) {
		m.Row(6, func() {
			for i, h := range t.Headers {
				if i >= len(t.Widths) {
					break
				}
				w := t.Widths[i]
				m.Col(w, func() {
					m.Text(h, props.Text{Top: 1, Style: consts.Bold, Size: 9, Align: consts.Left})
				})
			}
		})
	}
	// rows
	for _, row := range t.Rows {
		rowCopy := row
		m.Row(6, func() {
			style := consts.Normal
			if rowCopy.Bold {
				style = consts.Bold
			}
			for i, cell := range rowCopy.Cells {
				if i >= len(t.Widths) {
					break
				}
				w := t.Widths[i]
				align := consts.Left
				if i == len(rowCopy.Cells)-1 {
					align = consts.Right
				}
				cellText := cell
				m.Col(w, func() {
					tp := props.Text{Top: 1, Style: style, Size: 9, Align: align}
					if rowCopy.Red {
						tp.Color = redColor
					}
					m.Text(cellText, tp)
				})
			}
		})
	}
}

func hasNonEmpty(ss []string) bool {
	for _, s := range ss {
		if s != "" {
			return true
		}
	}
	return false
}
