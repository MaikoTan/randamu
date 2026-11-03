package pixiv

// Tag is a work tag.
type Tag struct {
	Name           string `json:"name"`
	TranslatedName string `json:"translated_name"`
}

// User is a work author.
type User struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// MetaPage is one page of a multi-page work.
type MetaPage struct {
	ImageURLs map[string]string `json:"image_urls"`
}

// SinglePage holds the original image information of a single-page work.
type SinglePage struct {
	OriginalImageURL string `json:"original_image_url"`
}

// Illust contains only the work fields the program needs.
type Illust struct {
	ID             int64             `json:"id"`
	Title          string            `json:"title"`
	Type           string            `json:"type"`
	User           User              `json:"user"`
	Tags           []Tag             `json:"tags"`
	IllustAIType   int               `json:"illust_ai_type"`
	XRestrict      int               `json:"x_restrict"`
	PageCount      int               `json:"page_count"`
	MetaSinglePage SinglePage        `json:"meta_single_page"`
	ImageURLs      map[string]string `json:"image_urls"`
	MetaPages      []MetaPage        `json:"meta_pages"`
}

// illustList is the response body of list endpoints.
type illustList struct {
	Illusts []Illust `json:"illusts"`
	NextURL string   `json:"next_url"`
}

// Page is the result of one API page.
type Page struct {
	Illusts []Illust
	NextURL string
}

// SearchParams are the parameters of a search source.
type SearchParams struct {
	Word         string
	SearchTarget string
	Sort         string
	Duration     string
}
