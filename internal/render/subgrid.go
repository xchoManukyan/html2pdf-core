package render

import "github.com/xchoManukyan/html2pdf-core/internal/htmlparse"

// Subgrid (CSS Grid Level 2): a grid item with grid-template-columns
// and/or grid-template-rows set to "subgrid" uses its parent's tracks in
// that axis. Its children take part in sizing the parent's tracks, so the
// rows of sibling cards line up, and the subgrid then lays its children out
// on the parent's track sizes.

// subTracks are the parent's track sizes a subgrid lays its items out on;
// nil for an axis that is not subgridded.
type subTracks struct {
	cols, rows     []float64
	colGap, rowGap float64
}

type subgridItem struct {
	gi         *gridItem
	cols, rows bool
	setup      *gridSetup // the subgrid's own placement
}

// boxExtras is margin + border + padding on each side (top, right, bottom,
// left): the space between a subgrid's edge tracks and its parent's lines.
func boxExtras(s *style) [4]float64 {
	var x [4]float64
	for i := range x {
		x[i] = s.margin[i] + s.bw(i) + s.padding[i] // bw: 0 when the border is not drawn
	}
	return x
}

// findSubgrids records the span of every subgrid item of g and places its
// own items.
func (e *engine) findSubgrids(g *gridSetup, w float64) []*subgridItem {
	var out []*subgridItem
	for _, gi := range g.items {
		if gi.st.display != "grid" && gi.st.display != "inline-grid" {
			continue
		}
		cols, rows := parseTrackList(gi.st.gridCols).subgrid, parseTrackList(gi.st.gridRows).subgrid
		if !cols && !rows {
			continue
		}
		if e.subgridSpan == nil {
			e.subgridSpan = map[*htmlparse.Node][2]int{}
			e.subgridTracks = map[*htmlparse.Node]*subTracks{}
		}
		var span [2]int
		if rows {
			span[0] = gi.rowSpan
		}
		if cols {
			span[1] = gi.colSpan
		}
		e.subgridSpan[gi.node] = span
		delete(e.subgridTracks, gi.node)
		out = append(out, &subgridItem{gi: gi, cols: cols, rows: rows, setup: e.setupGrid(gi.node, gi.st, w, true)})
	}
	return out
}

func (e *engine) subTracksOf(n *htmlparse.Node) *subTracks {
	t := e.subgridTracks[n]
	if t == nil {
		t = &subTracks{}
		e.subgridTracks[n] = t
	}
	return t
}

// columnSizingItems replaces column subgrids by their children, placed at
// their parent columns; extra is the subgrid's edge space an edge child adds.
func columnSizingItems(g *gridSetup, subs []*subgridItem) ([]*gridItem, map[*gridItem]float64) {
	extra := map[*gridItem]float64{}
	if len(subs) == 0 {
		return g.items, extra
	}
	skip := map[*gridItem]bool{}
	for _, s := range subs {
		if s.cols {
			skip[s.gi] = true
		}
	}
	var items []*gridItem
	for _, gi := range g.items {
		if !skip[gi] {
			items = append(items, gi)
		}
	}
	for _, s := range subs {
		if !s.cols {
			continue
		}
		x := boxExtras(s.gi.st)
		for _, ci := range s.setup.items {
			vi := &gridItem{flexItem: ci.flexItem, row: s.gi.row, rowSpan: s.gi.rowSpan, col: s.gi.col + ci.col, colSpan: ci.colSpan}
			if ci.col == 0 {
				extra[vi] += x[3]
			}
			if ci.col+ci.colSpan >= s.gi.colSpan {
				extra[vi] += x[1]
			}
			items = append(items, vi)
		}
	}
	return items, extra
}

// setSubgridColumns hands the parent's column sizes to its column subgrids.
func (e *engine) setSubgridColumns(subs []*subgridItem, colW []float64, colGap float64) {
	for _, s := range subs {
		if !s.cols {
			continue
		}
		x := boxExtras(s.gi.st)
		end := min(s.gi.col+s.gi.colSpan, len(colW))
		inner := append([]float64(nil), colW[s.gi.col:end]...)
		if len(inner) == 0 {
			continue
		}
		inner[0] = max(inner[0]-x[3], 0)
		inner[len(inner)-1] = max(inner[len(inner)-1]-x[1], 0)
		t := e.subTracksOf(s.gi.node)
		t.cols, t.colGap = inner, colGap
	}
}

// rowSizingItems replaces row subgrids by their children, each with its
// height measured at the width it gets inside the subgrid.
func (e *engine) rowSizingItems(g *gridSetup, subs []*subgridItem, areaW func(*gridItem) float64) ([]*gridItem, map[*gridItem]float64) {
	heights := map[*gridItem]float64{}
	skip := map[*gridItem]bool{}
	for _, s := range subs {
		if s.rows {
			skip[s.gi] = true
		}
	}
	if len(skip) == 0 {
		return g.items, heights
	}
	var items []*gridItem
	for _, gi := range g.items {
		if !skip[gi] {
			items = append(items, gi)
		}
	}
	for _, s := range subs {
		if !s.rows {
			continue
		}
		x := boxExtras(s.gi.st)
		// the subgrid's column sizes: its parent's (column subgrid) or its own
		var subColW []float64
		subGap := s.setup.colGap
		if t := e.subgridTracks[s.gi.node]; s.cols && t != nil && t.cols != nil {
			subColW, subGap = t.cols, t.colGap
		} else {
			cw := max(areaW(s.gi)-x[1]-x[3], 0)
			_, subColW = e.sizeTracks(s.setup.cols, s.setup.items, s.gi.st, cw, true, s.setup.colGap,
				func(gi *gridItem) (int, int) { return gi.col, gi.colSpan },
				func(gi *gridItem) (float64, float64) { return e.measureBlock(gi.node, gi.st) },
				s.gi.st.justify == "normal" || s.gi.st.justify == "stretch")
		}
		for _, ci := range s.setup.items {
			aw := trackSum(subColW, ci.col, ci.colSpan, subGap)
			bw := e.gridItemWidth(ci, s.gi.st, aw)
			h, _ := e.measureBox(ci.node, ci.st, bw)
			h += ci.st.margin[0] + ci.st.margin[2]
			if ci.row == 0 {
				h += x[0]
			}
			if ci.row+ci.rowSpan >= s.gi.rowSpan {
				h += x[2]
			}
			vi := &gridItem{flexItem: ci.flexItem, row: s.gi.row + ci.row, rowSpan: ci.rowSpan, col: s.gi.col, colSpan: s.gi.colSpan}
			heights[vi] = h
			items = append(items, vi)
		}
	}
	return items, heights
}

// setSubgridRows hands the parent's row sizes to its row subgrids.
func (e *engine) setSubgridRows(subs []*subgridItem, rowH []float64, rowGap float64) {
	for _, s := range subs {
		if !s.rows {
			continue
		}
		x := boxExtras(s.gi.st)
		end := min(s.gi.row+s.gi.rowSpan, len(rowH))
		inner := append([]float64(nil), rowH[s.gi.row:end]...)
		if len(inner) == 0 {
			continue
		}
		inner[0] = max(inner[0]-x[0], 0)
		inner[len(inner)-1] = max(inner[len(inner)-1]-x[2], 0)
		t := e.subTracksOf(s.gi.node)
		t.rows, t.rowGap = inner, rowGap
	}
}
