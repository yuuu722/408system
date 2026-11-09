package domain

import "testing"

func TestUnitCandidateOrderReservesFirstSlotForEachPoint(t *testing.T) {
	in := []PlanCandidate{{Asset: Asset{ID: "a1", PointID: "a"}}, {Asset: Asset{ID: "a2", PointID: "a"}}, {Asset: Asset{ID: "a3", PointID: "a"}}, {Asset: Asset{ID: "b1", PointID: "b"}}}
	out := UnitCandidateOrder(in)
	want := []string{"a1", "b1", "a2"}
	if len(out) != len(want) {
		t.Fatal(out)
	}
	for i, v := range want {
		if out[i].Asset.ID != v {
			t.Fatal(out)
		}
	}
}
