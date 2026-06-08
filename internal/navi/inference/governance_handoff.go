package inference

import "github.com/ceoai/navi/internal/governor"

// ActionDescriptor converts the handoff into the existing governor input shape.
func (g GovernanceHandoff) ActionDescriptor(chat ChatContext) governor.ActionDescriptor {
	return governor.ActionDescriptor{
		CommandType:      g.CommandType,
		Domain:           g.Domain,
		ActorKind:        firstNonEmpty(g.ActorKind, "navi"),
		ChatID:           chat.ChatID,
		RuntimeSessionID: chat.RuntimeSessionID,
		Tags:             cloneStringMap(g.Tags),
	}
}

func cloneStringMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]string, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}
