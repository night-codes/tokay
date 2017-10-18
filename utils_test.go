package tokay

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFilterFlags(t *testing.T) {
	result := filterFlags("text/html ")
	assert.Equal(t, result, "text/html")

	result = filterFlags("text/html;")
	assert.Equal(t, result, "text/html")
}
