package domain

import (
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestDiagnosticCoversSubjectsAndPointsBeforeRepeats(t *testing.T) {
	assets := []Asset{}
	subjects := map[string]string{}
	importance := map[string]float64{}
	for s := 0; s < 4; s++ {
		for p := 0; p < 3; p++ {
			point := fmt.Sprintf("%d-%d", s, p)
			subjects[point] = fmt.Sprint(s)
			importance[point] = float64(3 - p)
			for q := 0; q < 4; q++ {
				id := fmt.Sprintf("%s-%d", point, q)
				assets = append(assets, Asset{ID: id, Kind: "question", PointID: point, Status: "ready", Payload: AssetPayload{AnswerOrigin: "source", FamilyID: id}})
			}
		}
	}
	duplicate := assets[0]
	duplicate.ID = "copy"
	assets = append(assets, duplicate)
	selected := DiagnosticSelection(assets, subjects, importance, 12)
	require.Len(t, selected, 12)
	points, ss, families := map[string]bool{}, map[string]int{}, map[string]bool{}
	for _, a := range selected {
		require.False(t, families[Family(a)])
		families[Family(a)] = true
		points[a.PointID] = true
		ss[subjects[a.PointID]]++
	}
	require.Len(t, points, 12)
	require.Len(t, ss, 4)
	for _, n := range ss {
		require.Equal(t, 3, n)
	}
	require.Empty(t, DiagnosticSelection(nil, subjects, importance, 48))
}
