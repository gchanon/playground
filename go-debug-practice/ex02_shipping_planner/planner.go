package shipping

import (
	"fmt"
	"sort"
)

const handlingSKU = "HANDLING-FEE"

type Line struct {
	SKU      string
	Quantity int
	GramsEa  int
}

type Manifest struct {
	Number     int
	Zone       string
	Lines      []Line
	TotalGrams int
}

type Planner struct {
	batchSize   int
	zone        string
	maxGrams    int
	handlingEa  int
	includeFees bool
}

func NewPlanner(zone string, batchSize int) *Planner {
	return &Planner{
		batchSize:   batchSize,
		zone:        zone,
		maxGrams:    30000,
		handlingEa:  25,
		includeFees: true,
	}
}

func (p *Planner) handlingLine() Line {
	return Line{SKU: handlingSKU, Quantity: 1, GramsEa: p.handlingEa}
}

func (p *Planner) Plan(picklist []Line) ([]Manifest, error) {
	if p.batchSize <= 0 {
		return nil, fmt.Errorf("planner: batch size must be positive, got %d", p.batchSize)
	}

	sorted := make([]Line, len(picklist))
	copy(sorted, picklist)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].SKU < sorted[j].SKU
	})

	var manifests []Manifest
	for start := 0; start < len(sorted); start += p.batchSize {
		end := start + p.batchSize
		if end > len(sorted) {
			end = len(sorted)
		}

		batch := make([]Line, end-start)

		copy(batch, sorted[start:end])

		if p.includeFees {
			batch = append(batch, p.handlingLine())
		}

		m := Manifest{
			Number: len(manifests) + 1,
			Zone:   p.zone,
			Lines:  batch,
		}
		for _, l := range batch {
			m.TotalGrams += l.Quantity * l.GramsEa
		}
		if m.TotalGrams > p.maxGrams {
			return nil, fmt.Errorf("manifest %d: %dg exceeds zone limit %dg", m.Number, m.TotalGrams, p.maxGrams)
		}

		manifests = append(manifests, m)

		fmt.Println("check mnf round : ", manifests, ", start : ", start)

	}

	return manifests, nil
}

func PickedQuantities(manifests []Manifest) map[string]int {
	out := make(map[string]int)
	for _, m := range manifests {
		for _, l := range m.Lines {
			if l.SKU == handlingSKU {
				continue
			}
			out[l.SKU] += l.Quantity
		}
	}
	return out
}
