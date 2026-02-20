package knowledge

import (
	_ "embed"
	"fmt"
)

//go:embed self_knowledge.json
var selfKnowledge []byte

func CorePrompt() (string, error) {
	if len(selfKnowledge) == 0 {
		return "", fmt.Errorf("self knowledge is empty")
	}
	return string(selfKnowledge), nil
}
