package dto

type OpenAPIDocConfig struct {
	Title       string
	Version     string
	Description string
	ServerURL   string
}

type OpenAPIRoute struct {
	Method  string
	Path    string
	Handler string
}
