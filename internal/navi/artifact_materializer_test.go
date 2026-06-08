package navi

import (
	"testing"

	navitool "github.com/open-navi/navi/internal/tool"
	"github.com/stretchr/testify/assert"
)

func TestJudgeMaterialization(t *testing.T) {
	loop := &AgentLoop{}

	tests := []struct {
		name     string
		skill    string
		tool     string
		content  string
		expected bool
	}{
		{
			"Large Output",
			"filetools",
			"read_file",
			string(make([]byte, 3000)),
			true,
		},
		{
			"Small Output",
			"filetools",
			"read_file",
			"Hello world",
			false,
		},
		{
			"Markdown Table",
			"search",
			"web_search",
			"| Name | Age |\n|---|---|\n| Alice | 30 |",
			true,
		},
		{
			"Mermaid Diagram",
			"writer",
			"generate",
			"graph TD\nA-->B",
			true,
		},
		{
			"Search Summarize",
			"scout",
			"scout-search_summarize",
			"This is a summary of the search results.",
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tool := navitool.Tool{
				Name: tt.tool,
				Metadata: navitool.ToolMetadata{
					SkillName: tt.skill,
				},
			}
			judgment := loop.judgeMaterialization(tool, tt.content)
			assert.Equal(t, tt.expected, judgment.ShouldMaterialize)
		})
	}
}
