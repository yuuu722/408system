package domain

// Preserve priority within each point, but give other points a first learning
// slot before scheduling a second exercise for the same point.
func UnitCandidateOrder(in []PlanCandidate) []PlanCandidate {
	groups := map[string][]PlanCandidate{}
	order := []string{}
	for _, c := range in {
		if _, ok := groups[c.Asset.PointID]; !ok {
			order = append(order, c.Asset.PointID)
		}
		groups[c.Asset.PointID] = append(groups[c.Asset.PointID], c)
	}
	out := []PlanCandidate{}
	for round := 0; round < 2; round++ {
		for _, point := range order {
			if len(groups[point]) > round {
				out = append(out, groups[point][round])
			}
		}
	}
	return out
}
