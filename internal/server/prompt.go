// SPDX-License-Identifier: MIT

package server

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// PromptArg describes a single argument accepted by a prompt.
type PromptArg struct {
	Name        string
	Description string
	Required    bool
}

// PromptCount returns the number of prompts registered so far.
func (s *Server) PromptCount() int { return s.prompts }

// AddPrompt registers a prompt (a user-invoked, parameterized template that
// clients surface as a slash command). The render function builds the prompt
// text from the supplied argument values; the result is returned to the client
// as a single user message that guides the model through a multi-step flow.
func (s *Server) AddPrompt(name, description string, args []PromptArg, render func(args map[string]string) string) {
	margs := make([]*mcp.PromptArgument, 0, len(args))
	for _, a := range args {
		margs = append(margs, &mcp.PromptArgument{
			Name:        a.Name,
			Description: a.Description,
			Required:    a.Required,
		})
	}
	p := &mcp.Prompt{Name: name, Description: description, Arguments: margs}

	s.mcp.AddPrompt(p, func(_ context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		var values map[string]string
		if req != nil && req.Params != nil {
			values = req.Params.Arguments
		}
		return &mcp.GetPromptResult{
			Description: description,
			Messages: []*mcp.PromptMessage{
				{Role: "user", Content: &mcp.TextContent{Text: render(values)}},
			},
		}, nil
	})
	s.prompts++
}
