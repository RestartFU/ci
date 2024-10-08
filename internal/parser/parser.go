package parser

import (
	"fmt"
	"io"
	"math/rand"
	"os"
	"strconv"
	"strings"

	"github.com/restartfu/watch/internal/tokenizer"
)

type Checker struct {
	tokenizer *tokenizer.Tokenizer
	prevToken tokenizer.Token
	currToken tokenizer.Token

	variables map[string]string
	filename  string
}

func (c *Checker) Fatalf(pos tokenizer.Position, format string, args ...any) {
	fmt.Printf("%s(%d:%d)", c.filename, pos.Line(), pos.Column())
	fmt.Printf(format, args...)
	fmt.Println()
	os.Exit(1)
}

func (c *Checker) Next() (res tokenizer.Token) {
	token, err := c.tokenizer.Token()
	if err != nil && err != io.EOF {
		c.Fatalf(c.tokenizer.Position, " found invalid token: %v", err)
	}
	c.prevToken, c.currToken = c.currToken, token
	return c.prevToken
}

func (c *Checker) Expect(kind tokenizer.TokenKind) tokenizer.Token {
	token := c.Next()
	if token.Kind() != kind {
		c.Fatalf(token.Position, " expected token %v, got %v", kind, token.Kind())
	}
	return token
}

func (c *Checker) Allow(kind tokenizer.TokenKind) bool {
	if c.currToken.Kind() == kind {
		c.Next()
		return true
	}
	return false
}

func (c *Checker) Current() tokenizer.TokenKind {
	if c.currToken.Kind() == tokenizer.Comment {
		c.Next()
		return c.Current()
	}
	return c.currToken.Kind()
}

func (c *Checker) cloneDecl(dep *Result) {
	c.Next()
	tok := c.Expect(tokenizer.String)

	url := "https://" + tok.Text()
	tmp := "/tmp/watch-" + strconv.Itoa(rand.Intn(10000000))

	clone := fmt.Sprintf("git clone --depth=1 %s %s", url, tmp)

	split := strings.Split(url, "@")
	if len(split) == 2 {
		clone = fmt.Sprintf("git clone --depth=1 %s %s --branch %s", split[0], tmp, split[1])
	}
	cloning := []string{
		"cd /tmp",
		clone,
	}

	if c.Allow(tokenizer.As) {
		str := c.Expect(tokenizer.String)
		c.variables[str.Text()] = tmp
	}

	dep.Commands = append(dep.Commands, strings.Join(cloning, " && "))
}

func (c *Checker) runDecl(dep *Result) {
	c.Next()
	tok := c.Expect(tokenizer.String)
	str := tok.Text()
	for k, v := range c.variables {
		str = strings.ReplaceAll(str, fmt.Sprintf("$[%s]", k), v)
	}

	dep.Commands = append(dep.Commands, str)
}

func (c *Checker) extractDecl(dep *Result) {
	c.Next()
	tok := c.Expect(tokenizer.String)
	str := tok.Text()
	for k, v := range c.variables {
		str = strings.ReplaceAll(str, fmt.Sprintf("$[%s]", k), v)
	}

	split := strings.Split(str, " ")
	if len(split) != 2 {
		c.Fatalf(c.tokenizer.Position, " expected two arguments but got %d", len(split))
	}

	wd, _ := os.Getwd()
	out := split[1]
	if strings.HasPrefix(out, ".") {
		out = wd + out[1:]
	}

	cmd := fmt.Sprintf("mv %s %s ", split[0], out)
	dep.Commands = append(dep.Commands, cmd)
}

func (c *Checker) setDecl() {
	c.Next()
	tok := c.Expect(tokenizer.String)
	str := tok.Text()
	for k, v := range c.variables {
		str = strings.ReplaceAll(str, fmt.Sprintf("$[%s]", k), v)
	}

	split := strings.Split(str, "=")
	if len(split) != 2 {
		c.Fatalf(c.tokenizer.Position, " expected two arguments separated by '=' but got %d", len(split))
	}

	c.variables[split[0]] = split[1]
}

func Parse(filename string) []string {
	dep := &Result{}
	buf, err := os.ReadFile(filename)
	if err != nil {
		panic(err)
	}
	c := &Checker{
		tokenizer: tokenizer.NewTokenizer(string(buf)),
		variables: map[string]string{},
		filename:  filename,
	}
	c.Next()
decls:
	for {
		switch curr := c.Current(); curr {
		case tokenizer.Clone:
			c.cloneDecl(dep)
		case tokenizer.Run:
			c.runDecl(dep)
		case tokenizer.Extract:
			c.extractDecl(dep)
		case tokenizer.Set:
			c.setDecl()
		default:
			break decls
		}
	}
	c.Allow(tokenizer.Semicolon)
	c.Expect(tokenizer.EOF)

	return dep.Commands
}

type Result struct {
	Commands []string
}
