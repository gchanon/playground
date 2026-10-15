package orders

import "testing"

func TestReserveOnNewOrder(t *testing.T) {
	o := NewOrder("ORD-2001", "acme", "THB")
	if err := o.AddLine(Line{SKU: "WIDGET-1", Quantity: 5, UnitMinor: 1200}); err != nil {
		t.Fatalf("AddLine: %v", err)
	}
	if err := o.Reserve("WIDGET-1", 2); err != nil {
		t.Fatalf("Reserve: %v", err)
	}
	if got := o.ReservedCount("WIDGET-1"); got != 2 {
		t.Fatalf("ReservedCount = %d, want 2", got)
	}
}

func TestReserveOnDraftFromWarehouseQueue(t *testing.T) {
	raw := []byte(`{
		"id": "ORD-2002",
		"customer": "acme",
		"lines": [{"sku": "WIDGET-1", "quantity": 5, "unit_minor": 1200}]
	}`)

	o, err := LoadDraft(raw)
	if err != nil {
		t.Fatalf("LoadDraft: %v", err)
	}

	repo := NewRepository()
	repo.Save(o)

	if err := o.Reserve("WIDGET-1", 2); err != nil {
		t.Fatalf("Reserve: %v", err)
	}
	if got := o.ReservedCount("WIDGET-1"); got != 2 {
		t.Fatalf("ReservedCount = %d, want 2", got)
	}
	if err := o.Release("WIDGET-1", 2); err != nil {
		t.Fatalf("Release: %v", err)
	}
	if got := o.ReservedCount("WIDGET-1"); got != 0 {
		t.Fatalf("ReservedCount after release = %d, want 0", got)
	}
}
