package shipping

import (
	"reflect"
	"testing"
)

func picklist() []Line {
	return []Line{
		{SKU: "SKU-A", Quantity: 1, GramsEa: 100},
		{SKU: "SKU-B", Quantity: 2, GramsEa: 100},
		{SKU: "SKU-C", Quantity: 3, GramsEa: 100},
		{SKU: "SKU-D", Quantity: 4, GramsEa: 100},
		{SKU: "SKU-E", Quantity: 5, GramsEa: 100},
		{SKU: "SKU-F", Quantity: 6, GramsEa: 100},
		{SKU: "SKU-G", Quantity: 7, GramsEa: 100},
	}
}

func TestPlanShipsEveryPickedUnit(t *testing.T) {
	in := picklist()

	manifests, err := NewPlanner("BKK-NORTH", 3).Plan(in)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	want := map[string]int{}
	for _, l := range in {
		want[l.SKU] += l.Quantity
	}

	got := PickedQuantities(manifests)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("picked quantities = %v, want %v", got, want)
	}
}

func TestPlanDoesNotMutateInput(t *testing.T) {
	in := picklist()
	before := make([]Line, len(in))
	copy(before, in)

	if _, err := NewPlanner("BKK-NORTH", 3).Plan(in); err != nil {
		t.Fatalf("Plan: %v", err)
	}

	if !reflect.DeepEqual(in, before) {
		t.Fatalf("Plan mutated the caller's picklist:\n got  %v\n want %v", in, before)
	}
}
