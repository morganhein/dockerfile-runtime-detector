package tracker

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSearchTrackedProcesses(t *testing.T) {
	parent := &TrackedProcess{
		PID:      uint32(20),
		Children: make([]*TrackedProcess, 0),
	}
	child := &TrackedProcess{
		PID:      uint32(40),
		Children: make([]*TrackedProcess, 0),
	}
	parent.Children = append(parent.Children, child)

	searchParent := searchTrackedProcesses(parent, uint32(20))
	assert.NotNil(t, searchParent)
	assert.Equal(t, uint32(20), searchParent.PID)

	searchChild := searchTrackedProcesses(parent, uint32(40))
	assert.NotNil(t, searchParent)
	assert.Equal(t, uint32(40), searchChild.PID)

	searchNil := searchTrackedProcesses(parent, uint32(99))
	assert.Nil(t, searchNil)
}

// func TestPrintProcessTree(t *testing.T) {
// 	parent := &TrackedProcess{
// 		PID:      uint32(20),
// 		Children: make([]*TrackedProcess, 0),
// 		Start:    time.Now().Add(-30 * time.Second),
// 		End:      time.Now(),
// 	}
// 	child := &TrackedProcess{
// 		PID:      uint32(40),
// 		Children: make([]*TrackedProcess, 0),
// 		Start:    time.Now().Add(-20 * time.Second),
// 		End:      time.Now().Add(-10 * time.Second),
// 	}
// 	parent.Children = append(parent.Children, child)

// 	output := printProcessTree(parent, "")
// 	fmt.Println(output)
// 	assert.Contains(t, output, "PID:20")
// 	assert.Contains(t, output, "PID:20->PID:40")
// }
