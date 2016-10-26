package server

import (
	"strings"
	"bufio"
	"github.com/pkg/errors"
	"fmt"
	"zvr/utils"
)

type VyosParser struct {
	parsed bool
	tree *VyosConfigTree
}

type role int
const (
	ROOT role = iota
	ROOT_ATTRIBUTE
	KEY_VALUE
	CLOSE
	IGNORE
)

func matchToken(words []string) (int, role, []string, string) {
	ws := make([]string, 0)
	next := 0

	// find until \n
	for next = 0; next < len(words); next++ {
		w := words[next]
		if w == "\n" {
			break
		}

		ws = append(ws, w)
	}

	length := len(ws)
	if length == 2 && ws[length-1] == "{" {
		return next, ROOT, []string{ws[0]}, ""
	} else if  length > 2 && ws[length-1] == "{" {
		return next, ROOT_ATTRIBUTE, ws[:length-1], ""
	} else if length >= 2 && ws[length-1] != "{" && ws[length-1] != "}" {
		return next, KEY_VALUE, []string{ws[0]}, strings.Join(ws[1:], " ")
	} else if length == 1 && ws[0] == "}" {
		return next, CLOSE, nil, ""
	} else if length == 0 {
		return next+1, IGNORE, nil, ""
	} else {
		panic(errors.New(fmt.Sprintf("unable to parser the words: %s", strings.Join(words, " "))))
	}
}


func (parser *VyosParser) GetValue(key string) (string, bool) {
	if c, ok := parser.tree.Get(key); !ok {
		return "", false
	} else {
		return c.Value(), true
	}
}

func (parser *VyosParser) GetConfig(key string) (*VyosConfigNode, bool) {
	return parser.tree.Get(key)
}

func (parser *VyosParser) Parse(text string) *VyosConfigTree {
	parser.parsed = true

	words := make([]string, 0)
	for _, s := range strings.Split(text, "\n") {
		scanner := bufio.NewScanner(strings.NewReader(s))
		scanner.Split(bufio.ScanWords)
		ws := make([]string, 0)
		for scanner.Scan() {
			ws = append(ws, scanner.Text())
		}
		ws = append(ws, "\n")
		words = append(words, ws...)
	}

	offset := 0
	tree := &VyosConfigTree{ Root: &VyosConfigNode{} }
	tstack := &utils.Stack{}

	currentNode := tree.Root
	for i := 0; i < len(words); i += offset {
		o, role, keys, value := matchToken(words[i:])
		offset = o
		if role == ROOT {
			tstack.Push(currentNode)
			currentNode = currentNode.addNode(keys[0])
		} else if role == KEY_VALUE {
			currentNode.addNode(keys[0]).addNode(value)
		} else if role == ROOT_ATTRIBUTE {
			tstack.Push(currentNode)

			for _, key := range keys {
				if n := currentNode.getNode(key); n == nil {
					currentNode = currentNode.addNode(key)
				} else {
					currentNode = n
				}
			}
		} else if role == CLOSE {
			currentNode = tstack.Pop().(*VyosConfigNode)
		}
	}

	//txt, _ := json.Marshal(parser.data)
	//fmt.Println(string(txt))

	//fmt.Println(tree.String())
	parser.tree = tree
	return tree
}

var ConfigurationSourceFunc = func() string {
	bash := utils.Bash{
		Command: "/bin/cli-shell-api showCfg",
	}

	_, o, _, _ := bash.RunWithReturn()
	bash.PanicIfError()
	return o
}

func VyosShowConfiguration() string {
	return ConfigurationSourceFunc()
}

func NewParserFromShowConfiguration() *VyosParser {
	p := &VyosParser{}
	p.Parse(ConfigurationSourceFunc())
	return p
}

func NewParserFromConfiguration(text string) *VyosParser {
	p := &VyosParser{}
	p.Parse(text)
	return p
}

type VyosConfigNode struct {
	name          string
	children      []*VyosConfigNode
	childrenIndex map[string]*VyosConfigNode
	parent        *VyosConfigNode
}


func (n *VyosConfigNode) ChildNodeKeys() []string {
	keys := make([]string, 0)
	for k := range n.childrenIndex {
		keys = append(keys, k)
	}
	return keys
}

func (n *VyosConfigNode) String() string {
	stack := &utils.Stack{}
	p := n
	for {
		if p == nil {
			return func() string {
				sl := stack.Slice()
				ss := make([]string, len(sl))
				for i, s := range sl {
					ss[i] = s.(string)
				}
				return strings.Join(ss, " ")
			}()
		}

		stack.Push(p.name)
		p = p.parent
	}
}

func (n *VyosConfigNode) isValueNode() bool {
	return n.childrenIndex == nil && n.children == nil
}

func (n *VyosConfigNode) isKeyNode() bool {
	if len(n.children) != 1 {
		return false
	}

	c := n.children[0]
	return c.isValueNode()
}

func (n *VyosConfigNode) Value() string {
	utils.Assert(n.isKeyNode(), fmt.Sprintf("the node[%s] is not a key node", n.String()))

	c := n.children[0]
	return c.name
}

func (n *VyosConfigNode) deleteSelf() *VyosConfigNode {
	return n.parent.deleteNode(n.name)
}

func (n *VyosConfigNode) deleteNode(name string) *VyosConfigNode {
	delete(n.childrenIndex, name)
	nsl := make([]*VyosConfigNode, 0)
	for _, c := range n.children {
		if c.name != name {
			nsl = append(nsl, c)
		}
	}
	n.children = nsl
	return n
}

func (n *VyosConfigNode) getNode(name string) *VyosConfigNode {
	return n.childrenIndex[name]
}

func (n *VyosConfigNode) addNode(name string) *VyosConfigNode {
	if c, ok := n.childrenIndex[name]; ok {
		return c
	}

	newNode := &VyosConfigNode{
		name: name,
	}

	if n.children == nil {
		n.children = make([]*VyosConfigNode, 0)
	}
	n.children = append(n.children, newNode)

	if n.childrenIndex == nil {
		n.childrenIndex = make(map[string]*VyosConfigNode)
	}
	n.childrenIndex[name] = newNode
	newNode.parent = n
	return newNode
}


type VyosConfigTree struct {
	Root *VyosConfigNode
	changeCommands []string
}

func (t *VyosConfigTree) init() {
	if t.changeCommands == nil {
		t.changeCommands = make([]string, 0)
	}
	if t.Root == nil {
		t.Root = &VyosConfigNode{
			children: make([]*VyosConfigNode, 0),
			childrenIndex: make(map[string]*VyosConfigNode),
		}
	}
}

func (t *VyosConfigTree) has(config...string) bool {
	if t.Root == nil || t.Root.children == nil {
		return false
	}

	current := t.Root
	for _, c := range config {
		current = current.childrenIndex[c]
		if current == nil {
			return false
		}
	}

	return true
}

func (t *VyosConfigTree) Has(config string) bool {
	return t.has(strings.Split(config, " ")...)
}

func (t *VyosConfigTree) Set(config string) bool {
	t.init()
	cs := strings.Split(config, " ")
	key := strings.Join(cs[:len(cs)-1], " ")
	value := cs[len(cs)-1]
	keyNode, ok := t.Get(key)
	if ok {
		utils.Assert(keyNode.isKeyNode(), fmt.Sprintf("the node[%s] is not a key node, cannot call set on it", keyNode.String()))

		// the key found
		cvalue := keyNode.Value()
		if (value != cvalue) {
			keyNode.deleteNode(cvalue)
			keyNode.addNode(value)
			// the value is changed, delete the old one
			t.changeCommands = append(t.changeCommands, fmt.Sprintf("$DELETE %s", key))
			t.changeCommands = append(t.changeCommands, fmt.Sprintf("$SET %s", config))
			return true
		} else {
			// the value is unchanged
			return false
		}
	} else {
		// the key not found
		current := t.Root
		for _, c := range cs {
			current = current.addNode(c)
		}
		t.changeCommands = append(t.changeCommands, fmt.Sprintf("$SET %s", config))
		return true
	}
}

func (t *VyosConfigTree) Get(config string) (*VyosConfigNode, bool) {
	t.init()
	cs := strings.Split(config, " ")
	if !t.has(cs...) {
		return nil, false
	}

	current := t.Root
	for _, c := range cs {
		current = current.getNode(c)
	}

	return current, true
}

func (t *VyosConfigTree) Delete(config string) bool {
	current, ok := t.Get(config)
	if !ok {
		return false
	}

	current.deleteSelf()
	t.changeCommands = append(t.changeCommands, fmt.Sprintf("$DELETE %s", config))
	return true
}

func (t *VyosConfigTree) CommandsAsString() string {
	return strings.Join(t.changeCommands, "\n")
}

func (t *VyosConfigTree) Commands() []string {
	return t.changeCommands
}

func (t *VyosConfigTree) String() string {
	if t.Root == nil {
		return ""
	}

	strs := make([]string, 0)
	for _, n := range t.Root.children {
		path := utils.Stack{}

		var pathBuilder func(node *VyosConfigNode)
		pathBuilder = func(node *VyosConfigNode) {
			if node.children == nil {
				path.Push(node.name)
				strs = append(strs, func() string {
					sl := path.ReverseSlice()
					ss := make([]string, len(sl))
					for i, s := range sl {
						ss[i] = s.(string)
					}
					return strings.Join(ss, " ")
				}())
				path.Pop()

				return
			}

			path.Push(node.name)
			for _, cn := range node.children {
				pathBuilder(cn)
			}
			path.Pop()
		}

		pathBuilder(n)
	}

	return strings.Join(strs, "\n")
}

