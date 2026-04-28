package models

type BulkImportValidation struct {
	TotalProducts     int                      `json:"total_products"`
	ValidProducts     int                      `json:"valid_products"`
	InvalidProducts   int                      `json:"invalid_products"`
	MissingCategories []string                 `json:"missing_categories"`
	DuplicateSKUs     []string                 `json:"duplicate_skus"`
	Errors            []map[string]interface{} `json:"errors"`
	Warnings          []map[string]interface{} `json:"warnings"`
}

type BulkImportResult struct {
	InsertedCount int                      `json:"inserted_count"`
	ErrorsCount   int                      `json:"errors_count"`
	Errors        []map[string]interface{} `json:"errors"`
	Message       string                   `json:"message"`
	// RowResults contains per-row outcomes for the processed CSV.
	RowResults []map[string]interface{} `json:"row_results,omitempty"`
}
