package pattern

// ParsePattern parses a Core sexpr into Node IR (PatToNode).
func ParsePattern(s string) (Node, error) {
	p, err := ParseToPat(s)
	if err != nil {
		return Node{}, err
	}
	return PatToNode(p)
}

// groupArms returns alt bodies for a group node (Args, or legacy Callee).
func groupArms(n Node) []Node {
	if len(n.Args) > 0 {
		return n.Args
	}
	if n.Callee != nil {
		return []Node{*n.Callee}
	}
	return nil
}
