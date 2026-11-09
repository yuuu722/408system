package domain

import "testing"

func TestPointScopeAndDescendantCounts(t *testing.T) {
	points := []KnowledgePoint{{ID: "root"}, {ID: "child", ParentID: "root"}, {ID: "leaf", ParentID: "child"}, {ID: "other"}}
	scope := PointScope(points, "root")
	if len(scope) != 3 || scope["other"] {
		t.Fatal(scope)
	}
	count := SubtreeQuestionCounts(points, map[string]int{"root": 1, "child": 2, "leaf": 3})
	if count["root"] != 6 || count["child"] != 5 || count["leaf"] != 3 {
		t.Fatal(count)
	}
	cycle := []KnowledgePoint{{ID: "a", ParentID: "b"}, {ID: "b", ParentID: "a"}}
	if len(PointScope(cycle, "a")) != 2 {
		t.Fatal("cycle handling")
	}
	if SubtreeQuestionCounts(cycle, map[string]int{"a": 1})["a"] != 1 {
		t.Fatal("double counted root")
	}
}
