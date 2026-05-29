package build

import (
	"bytes"
	"fmt"
	"os/exec"
)

func printDependencyTree() (err error) {
	children := make(map[string][]string)
	visited := make(map[string]bool)

	cmd := exec.Command("go", "mod", "graph")
	out, err := cmd.CombinedOutput()
	if err != nil {
		err = fmt.Errorf("go mod: %w: %s", err, string(out))
		return
	}
	lines := bytes.SplitSeq(out, []byte("\n"))

	for line := range lines {
		if bytes.Equal(line, []byte("")) {
			continue
		}
		fields := bytes.Fields(line)
		if len(fields) != 2 {
			continue
		}
		node := string(fields[0])
		child := string(fields[1])
		children[node] = append(children[node], child)
	}

	cmd = exec.Command("go", "list", "-m")
	out, err = cmd.CombinedOutput()
	if err != nil {
		err = fmt.Errorf("go list: %w: %s", err, string(out))
		return
	}
	root := string(bytes.Trim(out, "\n"))

	var printTree func(node, prefix string)
	printTree = func(node, prefix string) {
		fmt.Printf("%s%s\n", prefix, node)

		visited[node] = true

		for _, child := range children[node] {
			if !visited[child] {
				printTree(child, "  "+prefix)
			} else {
				fmt.Printf("%s  %s (already shown)\n", prefix, child)
			}
		}
	}

	printTree(root, "")
	return
}
