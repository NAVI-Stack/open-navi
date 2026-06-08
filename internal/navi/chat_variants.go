package navi

type MessageVariant struct {
	ID      string `json:"id"`
	Index   int    `json:"index"`
	Content string `json:"content"`
}

type MessageVariants struct {
	Variants      []MessageVariant `json:"variants"`
	SelectedIndex int              `json:"selectedIndex"`
}
