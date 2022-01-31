package future

import (
	"testing"

	"github.com/stretchr/testify/require"
)

var fixtureExpectations = []expectation{
	// missing DNE.go and happy about that
	{
		"./DNE.go", missing{shouldMiss: true, msg: "Not ready to handle DNE.go"},
		"msg", missing{shouldMiss: true, msg: "Wow, we already have the struct msg"},
		[]string{"Subject", "Body"}, missing{shouldMiss: false, msg: "why don't we have Subject and Body"},
	},
	// missing DNE.go and un-happy about that
	{
		"./DNE.go", missing{shouldMiss: false, msg: "I really need DNE.go"},
		"ReallyImportStruct", missing{shouldMiss: false, msg: "I'm missing the ReallyImportStruct"},
		[]string{"field1", "field2"}, missing{shouldMiss: false, msg: "why don't we have field1 and field2"},
	},

	// have fixture.go and everything else, so super happy
	{
		"./fixture.go", missing{shouldMiss: false, msg: "should have fixture.go"},
		"msg", missing{shouldMiss: false, msg: "why don't we have struct msg?"},
		[]string{"Subject", "Body"}, missing{shouldMiss: false, msg: "why don't we have a Subjeee and Body"},
	},
	// have fixture.go, and un-happy about that only because fields messed up
	{
		"./fixture.go", missing{shouldMiss: false, msg: "should have fixture.go"},
		"msg", missing{shouldMiss: false, msg: "why don't we have struct msg?"},
		[]string{"Subjeee", "Body"}, missing{shouldMiss: false, msg: "why don't we have a Subjeee and Body"},
	},
	// have fixture.go, and un-happy about the fact that struct is missing
	{
		"./fixture.go", missing{shouldMiss: false, msg: "should have fixture.go"},
		"notMSG", missing{shouldMiss: false, msg: "why don't we have struct notMSG?"},
		[]string{"Subject", "Body"}, missing{shouldMiss: false, msg: "why don't we have a Subjeee and Body"},
	},
	// have fixture.go, and un-happy about that and everything else
	{
		"./fixture.go", missing{shouldMiss: true, msg: "Not ready to handle fixture.go"},
		"msg", missing{shouldMiss: true, msg: "Wow, we already have the struct msg"},
		[]string{"Subject", "Body"}, missing{shouldMiss: true, msg: "...and having Subject and Body makes life super difficult"},
	},
	// have fixture.go, and un-happy! But other stuff is just fine
	{
		"./fixture.go", missing{shouldMiss: true, msg: "Not ready to handle fixture.go"},
		"msg", missing{shouldMiss: false, msg: "Might as well have msg"},
		[]string{"Subject", "Body"}, missing{shouldMiss: false, msg: "...and having Subject and Body is fine too"},
	},
}

func TestFixture(t *testing.T) {
	problems := getProblematicExpectations(t, fixtureExpectations)

	require.Equal(t, 7, len(problems))

	require.Equal(t, 0, len(problems[0]))
	require.Equal(t, 1, len(problems[1]))
	require.Equal(t, 0, len(problems[2]))
	require.Equal(t, 1, len(problems[3]))
	require.Equal(t, 1, len(problems[4]))
	require.Equal(t, 3, len(problems[5]))
	require.Equal(t, 1, len(problems[6]))
}
