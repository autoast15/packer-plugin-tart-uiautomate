package uiauto

type Rect struct {
	X int `json:"x" mapstructure:"x"`
	Y int `json:"y" mapstructure:"y"`
	W int `json:"w" mapstructure:"w"`
	H int `json:"h" mapstructure:"h"`
}

func (r Rect) Center() (int, int) {
	return r.X + r.W/2, r.Y + r.H/2
}

type Screen struct {
	Width  int `json:"width" mapstructure:"width"`
	Height int `json:"height" mapstructure:"height"`
}

type Control struct {
	Role       string  `json:"role" mapstructure:"role"`
	Label      string  `json:"label" mapstructure:"label"`
	Value      string  `json:"value,omitempty" mapstructure:"value"`
	Selected   *bool   `json:"selected,omitempty" mapstructure:"selected"`
	Enabled    *bool   `json:"enabled,omitempty" mapstructure:"enabled"`
	BBox       Rect    `json:"bbox" mapstructure:"bbox"`
	Confidence float64 `json:"confidence" mapstructure:"confidence"`
}

type OCRItem struct {
	Text       string  `json:"text" mapstructure:"text"`
	BBox       Rect    `json:"bbox" mapstructure:"bbox"`
	Confidence float64 `json:"confidence" mapstructure:"confidence"`
}

type Detection struct {
	Screen   Screen    `json:"screen" mapstructure:"screen"`
	Controls []Control `json:"controls" mapstructure:"controls"`
	OCR      []OCRItem `json:"ocr" mapstructure:"ocr"`
}
