package zvr

import (
	"strings"
	"bufio"
	"github.com/pkg/errors"
	"fmt"
	"zvr/utils"
)

type VyosParser struct {
	data map[string]interface{}
	parsed bool
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
	} else if length == 2 && ws[length-1] != "{" && ws[length-1] != "}" {
		return next, KEY_VALUE, []string{ws[0]}, ws[1]
	} else if length == 1 && ws[0] == "}" {
		return next, CLOSE, nil, ""
	} else if length == 0 {
		return next+1, IGNORE, nil, ""
	} else {
		panic(errors.New(fmt.Sprintf("unable to parser the words: %s", strings.Join(words, " "))))
	}
}

type VyosConfig struct {
	data map[string]interface{}
}

func (c *VyosConfig) GetValue(keys ...string) (string, bool) {
	var current interface{} = c.data
	for _, key := range keys {
		m, ok := current.(map[string]interface{})
		if !ok {
			return "", false
		}

		current = m[key]
		if current == nil {
			return "", false
		}
	}

	if v, ok := current.(string); ok {
		return v, true
	} else {
		return "", false
	}
}

func (c *VyosConfig) GetConfig(keys ...string) (*VyosConfig, bool) {
	var current interface{} = c.data
	for _, key := range keys {
		m, ok := current.(map[string]interface{})
		if !ok {
			return nil, false
		}

		current = m[key]
		if current == nil {
			return nil, false
		}
	}

	if m, ok := current.(map[string]interface{}); ok {
		return &VyosConfig{
			data: m,
		}, true
	} else {
		return nil, false
	}
}

func (parser *VyosParser) GetValue(keys ...string) (string, bool) {
	if (len(keys) == 1) {
		utils.Assert(parser.parsed, "you must call Parse() before GetValue()")

	}

	mainKeys := keys[:len(keys)-1]
	if c, ok := parser.GetConfig(mainKeys...); ok {
		return c.GetValue([]string{keys[len(keys)-1]}...)
	} else {
		return "", false
	}
}

func (parser *VyosParser) GetConfig(keys ...string) (*VyosConfig, bool) {
	utils.Assert(parser.parsed, "you must call Parse() before GetConfig()")

	c := VyosConfig{ data: parser.data}
	return c.GetConfig(keys...)
}

func (parser *VyosParser) Parse(text string) {
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
	parser.data = make(map[string]interface{})
	var current map[string]interface{}
	for i := 0; i < len(words); i += offset {
		o, role, keys, value := matchToken(words[i:])
		offset = o
		if role == ROOT {
			current = make(map[string]interface{})
			parser.data[keys[0]] = current
		} else if role == KEY_VALUE {
			current[keys[0]] = value
		} else if role == ROOT_ATTRIBUTE {
			for _, key := range keys {
				current[key] = make(map[string]interface{})
				current = current[key].(map[string]interface{})
			}
		} else if role == CLOSE {
			current = make(map[string]interface{})
		}
	}
}

