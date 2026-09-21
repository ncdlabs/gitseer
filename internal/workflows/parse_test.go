package workflows

import "testing"

func TestParseNeedsDAG(t *testing.T) {
	yaml := []byte(`
name: CI
jobs:
  build:
    runs-on: ubuntu-latest
  test:
    needs: build
    runs-on: ubuntu-latest
  deploy:
    needs: [build, test]
    runs-on: ubuntu-latest
`)
	nodes, err := ParseNeedsDAG(yaml)
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 3 {
		t.Fatalf("nodes=%d", len(nodes))
	}
	byKey := map[string][]string{}
	for _, n := range nodes {
		byKey[n.JobKey] = n.Needs
	}
	if len(byKey["build"]) != 0 {
		t.Fatalf("build needs=%v", byKey["build"])
	}
	if len(byKey["test"]) != 1 || byKey["test"][0] != "build" {
		t.Fatalf("test needs=%v", byKey["test"])
	}
	if len(byKey["deploy"]) != 2 {
		t.Fatalf("deploy needs=%v", byKey["deploy"])
	}
}
