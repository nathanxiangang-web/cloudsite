package main

import "testing"

func TestOperatorPlanOnlyAcceptsDistinctIDsAndCloudSiteLogicalRoot(t *testing.T) {
	p := plan{"94000000-0000-4000-8000-000000000001", "94000000-0000-4000-8000-000000000002", "94000000-0000-4000-8000-000000000003", "/"}
	if validate(p) != nil {
		t.Fatal("valid plan")
	}
	p.SourcePath = "/outside"
	if validate(p) == nil {
		t.Fatal("scope bypass")
	}
	p.SourcePath = "/"
	p.BindingID = p.RootID
	if validate(p) == nil {
		t.Fatal("ID collision")
	}
}
