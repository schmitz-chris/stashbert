package api

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/oapi-codegen/nullable"

	"github.com/schmitz-chris/stashbert/internal/domain"
	"github.com/schmitz-chris/stashbert/internal/httpx"
)

// ListProducts returns all products with their barcodes, sorted by name.
func (s *Server) ListProducts(ctx context.Context, request ListProductsRequestObject) (ListProductsResponseObject, error) {
	rows, err := s.queries.ListProducts(ctx)
	if err != nil {
		return nil, fmt.Errorf("list products: %w", err)
	}
	barcodes, err := s.queries.ListBarcodes(ctx)
	if err != nil {
		return nil, fmt.Errorf("list barcodes: %w", err)
	}
	products, err := domain.ProductsFromDB(rows, barcodes)
	if err != nil {
		return nil, err
	}
	items := make([]Product, len(products))
	for i, p := range products {
		items[i] = productResponse(p)
	}
	return ListProducts200JSONResponse{Items: items}, nil
}

// GetProduct returns one product with its barcodes.
func (s *Server) GetProduct(ctx context.Context, request GetProductRequestObject) (GetProductResponseObject, error) {
	row, err := s.queries.GetProduct(ctx, request.Id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, httpx.NotFound("Produkt nicht gefunden")
	}
	if err != nil {
		return nil, fmt.Errorf("get product %s: %w", request.Id, err)
	}
	barcodes, err := s.queries.ListProductBarcodes(ctx, row.ID)
	if err != nil {
		return nil, fmt.Errorf("list barcodes of product %s: %w", row.ID, err)
	}
	p, err := domain.ProductFromDB(row, barcodes)
	if err != nil {
		return nil, err
	}
	return GetProduct200JSONResponse(productResponse(p)), nil
}

// CreateProduct creates a product manually and returns it with its location.
func (s *Server) CreateProduct(ctx context.Context, request CreateProductRequestObject) (CreateProductResponseObject, error) {
	body := request.Body
	in := domain.NewProduct{
		Name:        body.Name,
		Brand:       body.Brand,
		PackageSize: body.PackageSize,
		Note:        body.Note,
	}
	if body.Target != nil {
		in.Target = int64(*body.Target)
	}
	if body.MinStock != nil {
		in.MinStock = new(int64(*body.MinStock))
	}
	if body.Barcodes != nil {
		for _, b := range *body.Barcodes {
			units := int64(1)
			if b.Units != nil {
				units = int64(*b.Units)
			}
			in.Barcodes = append(in.Barcodes, domain.Barcode{Code: b.Code, Units: units})
		}
	}
	p, err := domain.CreateProduct(ctx, s.deps.DB, s.deps.Publisher, in)
	if err != nil {
		return nil, err
	}
	return CreateProduct201JSONResponse{
		Body:    productResponse(p),
		Headers: CreateProduct201ResponseHeaders{Location: "/api/v1/products/" + p.ID},
	}, nil
}

// UpdateProduct changes the fields given in the merge patch and returns the product.
func (s *Server) UpdateProduct(ctx context.Context, request UpdateProductRequestObject) (UpdateProductResponseObject, error) {
	body := request.Body
	patch := domain.ProductPatch{
		Name:        body.Name,
		Brand:       body.Brand,
		PackageSize: body.PackageSize,
		Note:        body.Note,
	}
	if body.Target != nil {
		patch.Target = new(int64(*body.Target))
	}
	switch {
	case body.MinStock.IsNull():
		patch.MinStock.SetNull()
	case body.MinStock.IsSpecified():
		patch.MinStock.Set(int64(body.MinStock.GetOrEmpty()))
	}
	p, err := domain.UpdateProduct(ctx, s.deps.DB, s.deps.Publisher, request.Id, patch)
	if err != nil {
		return nil, err
	}
	return UpdateProduct200JSONResponse(productResponse(p)), nil
}

// DeleteProduct deletes a product with its barcodes, movements and image file.
func (s *Server) DeleteProduct(ctx context.Context, request DeleteProductRequestObject) (DeleteProductResponseObject, error) {
	if err := domain.DeleteProduct(ctx, s.deps.DB, s.deps.Publisher, s.deps.ImageDir, request.Id); err != nil {
		return nil, err
	}
	return DeleteProduct204Response{}, nil
}

// MergeProduct merges a product into the target product, deletes it and
// returns the target.
func (s *Server) MergeProduct(ctx context.Context, request MergeProductRequestObject) (MergeProductResponseObject, error) {
	p, err := domain.MergeProduct(ctx, s.deps.DB, s.deps.Publisher, s.deps.ImageDir, request.Id, request.Body.TargetProductId)
	if err != nil {
		return nil, err
	}
	return MergeProduct200JSONResponse(productResponse(p)), nil
}

// GetProductImage returns the image file of a product.
func (s *Server) GetProductImage(ctx context.Context, request GetProductImageRequestObject) (GetProductImageResponseObject, error) {
	img, err := domain.OpenProductImage(ctx, s.deps.DB, s.deps.ImageDir, request.Id)
	if err != nil {
		return nil, err
	}
	// The generated response closes the file after writing it.
	return GetProductImage200ImageResponse{
		Body:          img.File,
		Headers:       GetProductImage200ResponseHeaders{CacheControl: "private, max-age=86400"},
		ContentType:   img.ContentType,
		ContentLength: img.Size,
	}, nil
}

// AddBarcode assigns a barcode to a product and returns it normalized.
func (s *Server) AddBarcode(ctx context.Context, request AddBarcodeRequestObject) (AddBarcodeResponseObject, error) {
	units := int64(1)
	if request.Body.Units != nil {
		units = int64(*request.Body.Units)
	}
	b, err := domain.AddBarcode(ctx, s.deps.DB, request.Id, domain.Barcode{Code: request.Body.Code, Units: units})
	if err != nil {
		return nil, err
	}
	return AddBarcode201JSONResponse{Code: b.Code, Units: int(b.Units)}, nil
}

// RemoveBarcode removes a barcode from a product.
func (s *Server) RemoveBarcode(ctx context.Context, request RemoveBarcodeRequestObject) (RemoveBarcodeResponseObject, error) {
	if err := domain.RemoveBarcode(ctx, s.deps.DB, request.Id, request.Code); err != nil {
		return nil, err
	}
	return RemoveBarcode204Response{}, nil
}

// productResponse maps p to the schema Product.
func productResponse(p domain.Product) Product {
	barcodes := make([]Barcode, len(p.Barcodes))
	for i, b := range p.Barcodes {
		barcodes[i] = Barcode{Code: b.Code, Units: int(b.Units)}
	}
	minStock := nullable.NewNullNullable[int]()
	if p.MinStock != nil {
		minStock.Set(int(*p.MinStock))
	}
	return Product{
		Id:          p.ID,
		Name:        p.Name,
		Brand:       toNullable(p.Brand),
		PackageSize: toNullable(p.PackageSize),
		Note:        toNullable(p.Note),
		Stock:       int(p.Stock),
		Target:      int(p.Target),
		MinStock:    minStock,
		Missing:     int(p.Missing),
		NeedsReview: p.NeedsReview,
		Origin:      ProductOrigin(p.Origin),
		LookupState: ProductLookupState(p.LookupState),
		HasImage:    p.HasImage,
		Barcodes:    barcodes,
		CreatedAt:   p.CreatedAt,
		UpdatedAt:   p.UpdatedAt,
	}
}

// toNullable returns null if v is nil and the value of v otherwise.
func toNullable[T any](v *T) nullable.Nullable[T] {
	if v == nil {
		return nullable.NewNullNullable[T]()
	}
	return nullable.NewNullableWithValue(*v)
}
