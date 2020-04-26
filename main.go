package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"reflect"
	"strings"
	"sync"
)

// Color of a redblack tree node is either
// `Black` (true) & `Red` (false)
type Color bool

// Direction points to either the Left or Right subtree
type Direction byte

func (c Color) String() string {
	switch c {
	case true:
		return "Black"
	default:
		return "Red"
	}
}

func (d Direction) String() string {
	switch d {
	case LEFT:
		return "left"
	case RIGHT:
		return "right"
	case NODIR:
		return "center"
	default:
		return "not recognized"
	}
}

const (
	BLACK, RED Color     = true, false
	LEFT       Direction = iota
	RIGHT
	NODIR
)

// A node needs to be able to answer the query:
// (i) Who is my parent node ?
// (ii) Who is my grandparent node ?
// The zero value for Node has color Red.
type Node struct {
	Key     interface{} `json:"key"`
	payload interface{}
	color   Color
	Left    *Node `json:"leftNode"`
	Right   *Node `json:"rightNode"`
	Leaf    bool  `json:"isLeaf"`
	parent  *Node
}

func (n *Node) String() string {
	return fmt.Sprintf("(%#v : %s)", n.Key, n.Color())
}

func (n *Node) Parent() *Node {
	return n.parent
}

func (n *Node) SetColor(color Color) {
	n.color = color
}

func (n *Node) Color() Color {
	return n.color
}

type Visitor interface {
	Visit(*Node)
}

// A redblack tree is `Visitable` by a `Visitor`.
type Visitable interface {
	Walk(Visitor)
}

// Keys must be comparable. It's mandatory to provide a Comparator,
// which returns zero if o1 == o2, -1 if o1 < o2, 1 if o1 > o2
type Comparator func(o1, o2 interface{}) int

// Default comparator expects keys to be of type `int`.
// Warning: if either one of `o1` or `o2` cannot be asserted to `int`, it panics.
func IntComparator(o1, o2 interface{}) int {
	i1 := o1.(int)
	i2 := o2.(int)
	switch {
	case i1 > i2:
		return 1
	case i1 < i2:
		return -1
	default:
		return 0
	}
}

// Keys of type `string`.
// Warning: if either one of `o1` or `o2` cannot be asserted to `string`, it panics.
func StringComparator(o1, o2 interface{}) int {
	s1 := o1.(string)
	s2 := o2.(string)
	return bytes.Compare([]byte(s1), []byte(s2))
}

// Tree encapsulates the data structure.
type Tree struct {
	Root *Node      `json:"root"`               // tip of the tree
	cmp  Comparator `json:"comparatorFunction"` // required function to order keys
}

// `lock` protects `logger`
var lock sync.Mutex
var logger *log.Logger

func init() {
	logger = log.New(ioutil.Discard, "", log.LstdFlags)
}

// TraceOn turns on logging output to Stderr
func TraceOn() {
	SetOutput(os.Stderr)
}

// TraceOff turns off logging.
// By default logging is turned off.
func TraceOff() {
	SetOutput(ioutil.Discard)
}

// SetOutput redirects log output
func SetOutput(w io.Writer) {
	lock.Lock()
	defer lock.Unlock()
	logger = log.New(w, "", log.LstdFlags)
}

// NewTree returns an empty Tree with default comparator `IntComparator`.
// `IntComparator` expects keys to be type-assertable to `int`.
func NewTree() *Tree {
	return &Tree{Root: nil, cmp: IntComparator}
}

// NewTreeWith returns an empty Tree with a supplied `Comparator`.
func NewTreeWith(c Comparator) *Tree {
	return &Tree{Root: nil, cmp: c}
}

// Get looks for the node with supplied key and returns its mapped payload.
// Return value in 1st position indicates whether any payload was found.
func (t *Tree) Get(key interface{}) (bool, interface{}) {
	if err := mustBeValidKey(key); err != nil {
		logger.Printf("Get was prematurely aborted: %s\n", err.Error())
		return false, nil
	}

	ok, node := t.getNode(key)
	if ok {
		return true, node.payload
	} else {
		return false, nil
	}
}

func (t *Tree) getNode(key interface{}) (bool, *Node) {
	found, parent, dir := t.GetParent(key)
	if found {
		if parent == nil {
			return true, t.Root
		} else {
			var node *Node
			switch dir {
			case LEFT:
				node = parent.Left
			case RIGHT:
				node = parent.Right
			}

			if node != nil {
				return true, node
			}
		}
	}
	return false, nil
}

// getMinimum returns the node with minimum key starting
// at the subtree rooted at node x. Assume x is not nil.
func (t *Tree) getMinimum(x *Node) *Node {
	for {
		if x.Left != nil {
			x = x.Left
		} else {
			return x
		}
	}
}

// GetParent looks for the node with supplied key and returns the parent node.
func (t *Tree) GetParent(key interface{}) (found bool, parent *Node, dir Direction) {
	if err := mustBeValidKey(key); err != nil {
		logger.Printf("GetParent was prematurely aborted: %s\n", err.Error())
		return false, nil, NODIR
	}

	if t.Root == nil {
		return false, nil, NODIR
	}

	return t.internalLookup(nil, t.Root, key, NODIR)
}

func (t *Tree) internalLookup(parent *Node, this *Node, key interface{}, dir Direction) (bool, *Node, Direction) {
	switch {
	case this == nil:
		return false, parent, dir
	case t.cmp(key, this.Key) == 0:
		return true, parent, dir
	case t.cmp(key, this.Key) < 0:
		return t.internalLookup(this, this.Left, key, LEFT)
	case t.cmp(key, this.Key) > 0:
		return t.internalLookup(this, this.Right, key, RIGHT)
	default:
		return false, parent, NODIR
	}
}

// Reverses actions of RotateLeft
func (t *Tree) RotateRight(y *Node) {
	if y == nil {
		logger.Printf("RotateRight: nil arg cannot be rotated. Noop\n")
		return
	}
	if y.Left == nil {
		logger.Printf("RotateRight: y has nil left subtree. Noop\n")
		return
	}
	logger.Printf("\t\t\trotate right of %s\n", y)
	x := y.Left
	y.Left = x.Right
	if x.Right != nil {
		x.Right.parent = y
	}
	x.parent = y.parent
	if y.parent == nil {
		t.Root = x
	} else {
		if y == y.parent.Left {
			y.parent.Left = x
		} else {
			y.parent.Right = x
		}
	}
	x.Right = y
	y.parent = x
}

// Side-effect: red-black tree properties is maintained.
func (t *Tree) RotateLeft(x *Node) {
	if x == nil {
		logger.Printf("RotateLeft: nil arg cannot be rotated. Noop\n")
		return
	}
	if x.Right == nil {
		logger.Printf("RotateLeft: x has nil right subtree. Noop\n")
		return
	}
	logger.Printf("\t\t\trotate left of %s\n", x)

	y := x.Right
	x.Right = y.Left
	if y.Left != nil {
		y.Left.parent = x
	}
	y.parent = x.parent
	if x.parent == nil {
		t.Root = y
	} else {
		if x == x.parent.Left {
			x.parent.Left = y
		} else {
			x.parent.Right = y
		}
	}
	y.Left = x
	x.parent = y
}

// Put saves the mapping (key, data) into the tree.
// If a mapping identified by `key` already exists, it is overwritten.
// Constraint: Not everything can be a key.
func (t *Tree) Put(key interface{}, data interface{}) error {
	if err := mustBeValidKey(key); err != nil {
		logger.Printf("Put was prematurely aborted: %s\n", err.Error())
		return err
	}

	if t.Root == nil {
		t.Root = &Node{Key: key, color: BLACK, payload: data}
		logger.Printf("Added %s as root node\n", t.Root.String())
		return nil
	}

	found, parent, dir := t.internalLookup(nil, t.Root, key, NODIR)
	if found {
		if parent == nil {
			logger.Printf("Put: parent=nil & found. Overwrite ROOT node\n")
			t.Root.payload = data
		} else {
			logger.Printf("Put: parent!=nil & found. Overwriting\n")
			switch dir {
			case LEFT:
				parent.Left.payload = data
			case RIGHT:
				parent.Right.payload = data
			}
		}

	} else {
		if parent != nil {
			newNode := &Node{Key: key, parent: parent, payload: data}
			switch dir {
			case LEFT:
				parent.Left = newNode
			case RIGHT:
				parent.Right = newNode
			}
			logger.Printf("Added %s to %s node of parent %s\n", newNode.String(), dir, parent.String())
			t.fixupPut(newNode)
		}
	}
	return nil
}

func isRed(n *Node) bool {
	key := reflect.ValueOf(n)
	if key.IsNil() {
		return false
	} else {
		return n.color == RED
	}
}

// fix possible violations of red-black-tree properties
// with combinations of:
// 1. recoloring
// 2. rotations
//
// Preconditions:
// P1) z is not nil
//
// @param z - the newly added Node to the tree.
func (t *Tree) fixupPut(z *Node) {
	logger.Printf("\tfixup new node z %s\n", z.String())
loop:
	for {
		logger.Printf("\tcurrent z %s\n", z.String())
		switch {
		case z.parent == nil:
			fallthrough
		case z.parent.color == BLACK:
			fallthrough
		default:
			// When the loop terminates, it does so because p[z] is black.
			logger.Printf("\t\t=> bye\n")
			break loop
		case z.parent.color == RED:
			grandparent := z.parent.parent
			logger.Printf("\t\tgrandparent is nil %t\n", grandparent == nil)
			if z.parent == grandparent.Left {
				logger.Printf("\t\t%s is the left child of %s\n", z.parent, grandparent)
				y := grandparent.Right
				logger.Printf("\t\ty (right) %s\n", y)
				if isRed(y) {
					// case 1 - y is RED
					logger.Printf("\t\t(*) case 1\n")
					z.parent.color = BLACK
					y.color = BLACK
					grandparent.color = RED
					z = grandparent

				} else {
					if z == z.parent.Right {
						// case 2
						logger.Printf("\t\t(*) case 2\n")
						z = z.parent
						t.RotateLeft(z)
					}

					// case 3
					logger.Printf("\t\t(*) case 3\n")
					z.parent.color = BLACK
					grandparent.color = RED
					t.RotateRight(grandparent)
				}
			} else {
				logger.Printf("\t\t%s is the right child of %s\n", z.parent, grandparent)
				y := grandparent.Left
				logger.Printf("\t\ty (left) %s\n", y)
				if isRed(y) {
					// case 1 - y is RED
					logger.Printf("\t\t..(*) case 1\n")
					z.parent.color = BLACK
					y.color = BLACK
					grandparent.color = RED
					z = grandparent

				} else {
					logger.Printf("\t\t## %s\n", z.parent.Left)
					if z == z.parent.Left {
						// case 2
						logger.Printf("\t\t..(*) case 2\n")
						z = z.parent
						t.RotateRight(z)
					}

					// case 3
					logger.Printf("\t\t..(*) case 3\n")
					z.parent.color = BLACK
					grandparent.color = RED
					t.RotateLeft(grandparent)
				}
			}
		}
	}
	t.Root.color = BLACK
}

// Size returns the number of items in the tree.
func (t *Tree) Size() uint64 {
	visitor := &countingVisitor{}
	t.Walk(visitor)
	return visitor.Count
}

// Has checks for existence of a item identified by supplied key.
func (t *Tree) Has(key interface{}) bool {
	if err := mustBeValidKey(key); err != nil {
		logger.Printf("Has was prematurely aborted: %s\n", err.Error())
		return false
	}
	found, _, _ := t.internalLookup(nil, t.Root, key, NODIR)
	return found
}

func (t *Tree) transplant(u *Node, v *Node) {
	if u.parent == nil {
		t.Root = v
	} else if u == u.parent.Left {
		u.parent.Left = v
	} else {
		u.parent.Right = v
	}
	if v != nil && u != nil {
		v.parent = u.parent
	}
}

// Delete removes the item identified by the supplied key.
// Delete is a noop if the supplied key doesn't exist.
func (t *Tree) Delete(key interface{}) {
	if !t.Has(key) {
		logger.Printf("Delete: bail as no node exists for key %d\n", key)
		return
	}
	_, z := t.getNode(key)
	logger.Printf("Delete: attempt to delete %s\n", z)
	y := z
	yOriginalColor := y.color
	var x *Node

	if z.Left == nil {
		// one child (RIGHT)
		logger.Printf("\t\tDelete: case (a)\n")
		x = z.Right
		logger.Printf("\t\t\t--- x is right of z")
		t.transplant(z, z.Right)

	} else if z.Right == nil {
		// one child (LEFT)
		logger.Printf("\t\tDelete: case (b)\n")
		x = z.Left
		logger.Printf("\t\t\t--- x is left of z")
		t.transplant(z, z.Left)

	} else {
		// two children
		logger.Printf("\t\tDelete: case (c) & (d)\n")
		y = t.getMinimum(z.Right)
		logger.Printf("\t\t\tminimum of z.Right is %s (color=%s)\n", y, y.color)
		yOriginalColor = y.color
		x = y.Right
		logger.Printf("\t\t\t--- x is right of minimum")

		if y.parent == z {
			if x != nil {
				x.parent = y
			}
		} else {
			t.transplant(y, y.Right)
			y.Right = z.Right
			y.Right.parent = y
		}
		t.transplant(z, y)
		y.Left = z.Left
		y.Left.parent = y
		y.color = z.color
	}
	if yOriginalColor == BLACK {
		t.fixupDelete(x)
	}
}

func (t *Tree) fixupDelete(x *Node) {
	logger.Printf("\t\t\tfixupDelete of node %s\n", x)
	if x == nil {
		return
	}
loop:
	for {
		switch {
		case x == t.Root:
			logger.Printf("\t\t\t=> bye .. is root\n")
			break loop
		case x.color == RED:
			logger.Printf("\t\t\t=> bye .. RED\n")
			break loop
		case x == x.parent.Right:
			logger.Printf("\t\tBRANCH: x is right child of parent\n")
			w := x.parent.Left // is nillable
			if isRed(w) {
				// Convert case 1 into case 2, 3, or 4
				logger.Printf("\t\t\tR> case 1\n")
				w.color = BLACK
				x.parent.color = RED
				t.RotateRight(x.parent)
				w = x.parent.Left
			}
			if w != nil {
				switch {
				case !isRed(w.Left) && !isRed(w.Right):
					// case 2 - both children of w are BLACK
					logger.Printf("\t\t\tR> case 2\n")
					w.color = RED
					x = x.parent // recurse up tree
				case isRed(w.Right) && !isRed(w.Left):
					// case 3 - right child RED & left child BLACK
					// convert to case 4
					logger.Printf("\t\t\tR> case 3\n")
					w.Right.color = BLACK
					w.color = RED
					t.RotateLeft(w)
					w = x.parent.Left
				}
				if isRed(w.Left) {
					// case 4 - left child is RED
					logger.Printf("\t\t\tR> case 4\n")
					w.color = x.parent.color
					x.parent.color = BLACK
					w.Left.color = BLACK
					t.RotateRight(x.parent)
					x = t.Root
				}
			}
		case x == x.parent.Left:
			logger.Printf("\t\tBRANCH: x is left child of parent\n")
			w := x.parent.Right // is nillable
			if isRed(w) {
				// Convert case 1 into case 2, 3, or 4
				logger.Printf("\t\t\tL> case 1\n")
				w.color = BLACK
				x.parent.color = RED
				t.RotateLeft(x.parent)
				w = x.parent.Right
			}
			if w != nil {
				switch {
				case !isRed(w.Left) && !isRed(w.Right):
					// case 2 - both children of w are BLACK
					logger.Printf("\t\t\tL> case 2\n")
					w.color = RED
					x = x.parent // recurse up tree
				case isRed(w.Left) && !isRed(w.Right):
					// case 3 - left child RED & right child BLACK
					// convert to case 4
					logger.Printf("\t\t\tL> case 3\n")
					w.Left.color = BLACK
					w.color = RED
					t.RotateRight(w)
					w = x.parent.Right
				}
				if isRed(w.Right) {
					// case 4 - right child is RED
					logger.Printf("\t\t\tL> case 4\n")
					w.color = x.parent.color
					x.parent.color = BLACK
					w.Right.color = BLACK
					t.RotateLeft(x.parent)
					x = t.Root
				}
			}
		}
	}
	x.color = BLACK
}

// Walk accepts a Visitor
func (t *Tree) Walk(visitor Visitor) {
	visitor.Visit(t.Root)
}

// countingVisitor counts the number
// of nodes in the tree.
type countingVisitor struct {
	Count uint64
}

func (v *countingVisitor) Visit(node *Node) {
	if node == nil {
		return
	}

	v.Visit(node.Left)
	v.Count = v.Count + 1
	v.Visit(node.Right)
}

// InorderVisitor walks the tree in inorder fashion.
// This visitor maintains internal state; thus do not
// reuse after the completion of a walk.
type InorderVisitor struct {
	buffer bytes.Buffer
}

func (v *InorderVisitor) Eq(other *InorderVisitor) bool {
	if other == nil {
		return false
	}
	return v.String() == other.String()
}

func (v *InorderVisitor) trim(s string) string {
	return strings.TrimRight(strings.TrimRight(s, "ed"), "lack")
}

func (v *InorderVisitor) String() string {
	return v.buffer.String()
}

func (v *InorderVisitor) Visit(node *Node) {
	if node == nil {
		v.buffer.Write([]byte("."))
		return
	}
	v.buffer.Write([]byte("("))
	v.Visit(node.Left)
	v.buffer.Write([]byte(fmt.Sprintf("%d", node.Key))) // @TODO
	//v.buffer.Write([]byte(fmt.Sprintf("%d{%s}", node.Key, v.trim(node.color.String()))))
	v.Visit(node.Right)
	v.buffer.Write([]byte(")"))
}

var (
	ErrorKeyIsNil      = errors.New("The literal nil not allowed as keys")
	ErrorKeyDisallowed = errors.New("Disallowed key type")
)

func mustBeValidKey(key interface{}) error {
	if key == nil {
		return ErrorKeyIsNil
	}

	keyValue := reflect.ValueOf(key)
	switch keyValue.Kind() {
	case reflect.Chan:
		fallthrough
	case reflect.Func:
		fallthrough
	case reflect.Interface:
		fallthrough
	case reflect.Map:
		fallthrough
	case reflect.Ptr:
		fallthrough
	case reflect.Slice:
		return ErrorKeyDisallowed
	default:
		return nil
	}
}

func getSplitNode(n *Node, x1, x2 int, debug bool) *Node {

	if n.Key.(int) >= x1 && n.Key.(int) <= x2 {
		if debug {
			log.Printf("[SUCCESS] - Found Split Node: %+v", n.String())
		}
		return n
	}

	if n.Left != nil {
		return getSplitNode(n.Left, x1, x2, debug)
	}

	if n.Right != nil {
		return getSplitNode(n.Right, x1, x2, debug)
	}
	return nil
}

func (n *Node) isLeaf() bool {
	if n.Right == nil && n.Left == nil {
		return true
	}
	return false
}

func (t *Tree) getValuesInRange(x1, x2 int, debug bool) []int {
	if debug {
		log.Printf("[Query] Values between %v and %v", x1, x2)
	}
	rangeNodes := []Node{}
	Vs := getSplitNode(t.Root, x1, x2, debug)
	if Vs == nil {
		log.Printf("\n\t[ERR] Couldn't find Split Node\n")
		return []int{}
	}

	curNode := Vs
	if curNode.isLeaf() {
		if curNode.Key.(int) >= x1 && curNode.Key.(int) <= x2 {
			rangeNodes = append(rangeNodes, *curNode)
		}
	} else {
		curNode = curNode.Left
	}

	/*Going left*/

	for true {
		if !curNode.isLeaf() {

			if x1 <= curNode.Key.(int) {
				rangeNodes = append(rangeNodes, *curNode.Right)
				curNode = curNode.Left
			} else {
				curNode = curNode.Right
			}

		} else {
			break
		}
	}

	if curNode.Key.(int) >= x1 && curNode.Key.(int) <= x2 {
		rangeNodes = append(rangeNodes, *curNode)
	}

	/*Going right*/

	curNode = Vs.Right
	for true {
		if !curNode.isLeaf() {
			if curNode.Key.(int) <= x2 {
				rangeNodes = append(rangeNodes, *curNode.Left)
				curNode = curNode.Right
			} else {
				curNode = curNode.Left
			}
		} else {
			break
		}
	}

	if curNode.Key.(int) >= x1 && curNode.Key.(int) <= x2 {
		rangeNodes = append(rangeNodes, *curNode)
	}
	keys := []int{}
	for _, node := range rangeNodes {
		keys = append(keys, node.Key.(int))
	}

	log.Printf("Values in Range [%v, %v] -> %+v", x1, x2, keys)
	return keys
}

func printToJSON(t *Tree) {
	/* Print JSON to file */
	file, _ := json.MarshalIndent(t, "", " ")
	_ = ioutil.WriteFile("tree.json", file, 0644)
}

func main() {
	leaf3 := &Node{Key: 3, Leaf: true}
	leaf10 := &Node{Key: 10, Leaf: true}
	leaf19 := &Node{Key: 19, Leaf: true}
	leaf23 := &Node{Key: 23, Leaf: true}
	leaf30 := &Node{Key: 30, Leaf: true}
	leaf37 := &Node{Key: 37, Leaf: true}
	leaf49 := &Node{Key: 49, Leaf: true}
	leaf59 := &Node{Key: 59, Leaf: true}
	leaf62 := &Node{Key: 62, Leaf: true}
	leaf70 := &Node{Key: 70, Leaf: true}
	leaf80 := &Node{Key: 80, Leaf: true}
	leaf100 := &Node{Key: 100, Leaf: true}

	node3 := Node{Key: 3, Left: leaf3, Right: leaf10}
	node19 := Node{Key: 19, Left: leaf19, Right: leaf23}
	node30 := Node{Key: 30, Left: leaf30, Right: leaf37}
	node59 := Node{Key: 59, Left: leaf59, Right: leaf62}
	node70 := Node{Key: 70, Left: leaf70, Right: leaf80}
	node100 := Node{Key: 100, Left: leaf100}

	node10 := Node{Key: 10, Left: &node3, Right: &node19}
	node37 := Node{Key: 37, Left: &node30, Right: leaf49}
	node62 := Node{Key: 62, Left: &node59, Right: &node70}
	node89 := Node{Key: 89, Right: &node100}

	node23 := Node{Key: 23, Left: &node10, Right: &node37}
	node80 := Node{Key: 80, Left: &node62, Right: &node89}

	tree := Tree{Root: &Node{Key: 49, Left: &node23, Right: &node80}, cmp: IntComparator}

	printToJSON(&tree)

	/* Range TESTS */
	_ = tree.getValuesInRange(19, 77, false)

}
