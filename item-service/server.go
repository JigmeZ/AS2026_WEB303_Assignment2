package main

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "lostfound/gen/itempb"
)

var validStatuses = map[string]bool{"FOUND": true, "CLAIMED": true, "RETURNED": true}

// server implements pb.ItemServiceServer.
type server struct {
	pb.UnimplementedItemServiceServer
	db *sql.DB
}

// ---------- validation ----------

func validateItemFields(name, description, location string) error {
	name = strings.TrimSpace(name)
	location = strings.TrimSpace(location)
	switch {
	case name == "":
		return status.Error(codes.InvalidArgument, "name is required")
	case len(name) > 100:
		return status.Error(codes.InvalidArgument, "name must be at most 100 characters")
	case location == "":
		return status.Error(codes.InvalidArgument, "location_found is required")
	case len(location) > 200:
		return status.Error(codes.InvalidArgument, "location_found must be at most 200 characters")
	case len(description) > 500:
		return status.Error(codes.InvalidArgument, "description must be at most 500 characters")
	}
	return nil
}

func validateID(id int64) error {
	if id <= 0 {
		return status.Error(codes.InvalidArgument, "id must be a positive integer")
	}
	return nil
}

// ---------- CRUD ----------

func (s *server) CreateItem(ctx context.Context, req *pb.CreateItemRequest) (*pb.Item, error) {
	if err := validateItemFields(req.GetName(), req.GetDescription(), req.GetLocationFound()); err != nil {
		return nil, err
	}
	item := &pb.Item{
		Name:          strings.TrimSpace(req.GetName()),
		Description:   strings.TrimSpace(req.GetDescription()),
		LocationFound: strings.TrimSpace(req.GetLocationFound()),
		Status:        "FOUND",
	}
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO items (name, description, location_found, status)
		 VALUES ($1, $2, $3, $4) RETURNING id`,
		item.Name, item.Description, item.LocationFound, item.Status,
	).Scan(&item.Id)
	if err != nil {
		log.Printf("CreateItem DB error: %v", err)
		return nil, status.Error(codes.Internal, "failed to create item")
	}
	return item, nil
}

func (s *server) GetItem(ctx context.Context, req *pb.GetItemRequest) (*pb.Item, error) {
	if err := validateID(req.GetId()); err != nil {
		return nil, err
	}
	item := &pb.Item{}
	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, description, location_found, status FROM items WHERE id = $1`,
		req.GetId(),
	).Scan(&item.Id, &item.Name, &item.Description, &item.LocationFound, &item.Status)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, status.Errorf(codes.NotFound, "item %d not found", req.GetId())
	}
	if err != nil {
		log.Printf("GetItem DB error: %v", err)
		return nil, status.Error(codes.Internal, "failed to retrieve item")
	}
	return item, nil
}

func (s *server) UpdateItem(ctx context.Context, req *pb.UpdateItemRequest) (*pb.Item, error) {
	if err := validateID(req.GetId()); err != nil {
		return nil, err
	}
	if err := validateItemFields(req.GetName(), req.GetDescription(), req.GetLocationFound()); err != nil {
		return nil, err
	}
	if !validStatuses[req.GetStatus()] {
		return nil, status.Error(codes.InvalidArgument, "status must be one of FOUND, CLAIMED, RETURNED")
	}

	item := &pb.Item{}
	err := s.db.QueryRowContext(ctx,
		`UPDATE items SET name = $1, description = $2, location_found = $3, status = $4
		 WHERE id = $5
		 RETURNING id, name, description, location_found, status`,
		strings.TrimSpace(req.GetName()), strings.TrimSpace(req.GetDescription()),
		strings.TrimSpace(req.GetLocationFound()), req.GetStatus(), req.GetId(),
	).Scan(&item.Id, &item.Name, &item.Description, &item.LocationFound, &item.Status)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, status.Errorf(codes.NotFound, "item %d not found", req.GetId())
	}
	if err != nil {
		log.Printf("UpdateItem DB error: %v", err)
		return nil, status.Error(codes.Internal, "failed to update item")
	}
	return item, nil
}

func (s *server) DeleteItem(ctx context.Context, req *pb.DeleteItemRequest) (*pb.DeleteItemResponse, error) {
	if err := validateID(req.GetId()); err != nil {
		return nil, err
	}
	res, err := s.db.ExecContext(ctx, `DELETE FROM items WHERE id = $1`, req.GetId())
	if err != nil {
		log.Printf("DeleteItem DB error: %v", err)
		return nil, status.Error(codes.Internal, "failed to delete item")
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, status.Errorf(codes.NotFound, "item %d not found", req.GetId())
	}
	return &pb.DeleteItemResponse{Message: "item deleted"}, nil
}

func (s *server) ListItems(ctx context.Context, _ *pb.ListItemsRequest) (*pb.ListItemsResponse, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, description, location_found, status FROM items ORDER BY id`)
	if err != nil {
		log.Printf("ListItems DB error: %v", err)
		return nil, status.Error(codes.Internal, "failed to list items")
	}
	defer rows.Close()

	resp := &pb.ListItemsResponse{}
	for rows.Next() {
		item := &pb.Item{}
		if err := rows.Scan(&item.Id, &item.Name, &item.Description, &item.LocationFound, &item.Status); err != nil {
			log.Printf("ListItems scan error: %v", err)
			return nil, status.Error(codes.Internal, "failed to list items")
		}
		resp.Items = append(resp.Items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, status.Error(codes.Internal, "failed to list items")
	}
	return resp, nil
}
