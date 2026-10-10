package main

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"regexp"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "lostfound/gen/claimpb"
)

var validStatuses = map[string]bool{"PENDING": true, "APPROVED": true, "REJECTED": true}
var emailRegex = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

// server implements pb.ClaimServiceServer.
type server struct {
	pb.UnimplementedClaimServiceServer
	db    *sql.DB
	items *ItemClient // the only way this service may access item data
}

// ---------- validation ----------

func validateID(id int64, field string) error {
	if id <= 0 {
		return status.Errorf(codes.InvalidArgument, "%s must be a positive integer", field)
	}
	return nil
}

func validateClaimant(name, email string) error {
	name = strings.TrimSpace(name)
	email = strings.TrimSpace(email)
	switch {
	case name == "":
		return status.Error(codes.InvalidArgument, "claimant_name is required")
	case len(name) > 100:
		return status.Error(codes.InvalidArgument, "claimant_name must be at most 100 characters")
	case email == "":
		return status.Error(codes.InvalidArgument, "claimant_email is required")
	case len(email) > 200 || !emailRegex.MatchString(email):
		return status.Error(codes.InvalidArgument, "claimant_email is not a valid email address")
	}
	return nil
}

// ---------- CRUD ----------

// CreateClaim validates input, then asks Service B (over gRPC) whether the item
// exists. The claim is only stored if the item exists and is still claimable.
func (s *server) CreateClaim(ctx context.Context, req *pb.CreateClaimRequest) (*pb.Claim, error) {
	if err := validateID(req.GetItemId(), "item_id"); err != nil {
		return nil, err
	}
	if err := validateClaimant(req.GetClaimantName(), req.GetClaimantEmail()); err != nil {
		return nil, err
	}

	item, err := s.items.GetItem(ctx, req.GetItemId())
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, status.Errorf(codes.NotFound, "cannot create claim: item %d does not exist", req.GetItemId())
		}
		return nil, err // Unavailable / DeadlineExceeded etc. are passed to the gateway
	}
	if item.GetStatus() == "RETURNED" {
		return nil, status.Errorf(codes.FailedPrecondition, "item %d has already been returned to its owner", item.GetId())
	}

	claim := &pb.Claim{
		ItemId:        req.GetItemId(),
		ClaimantName:  strings.TrimSpace(req.GetClaimantName()),
		ClaimantEmail: strings.TrimSpace(req.GetClaimantEmail()),
		Status:        "PENDING",
		ItemName:      item.GetName(),
	}
	err = s.db.QueryRowContext(ctx,
		`INSERT INTO claims (item_id, claimant_name, claimant_email, status)
		 VALUES ($1, $2, $3, $4) RETURNING id`,
		claim.ItemId, claim.ClaimantName, claim.ClaimantEmail, claim.Status,
	).Scan(&claim.Id)
	if err != nil {
		log.Printf("CreateClaim DB error: %v", err)
		return nil, status.Error(codes.Internal, "failed to create claim")
	}
	return claim, nil
}

// GetClaim reads the claim from its own DB, then enriches it with the item name
// from Service B. If Service B is down, it returns a FALLBACK response (the claim
// itself, with a placeholder item name) instead of failing.
func (s *server) GetClaim(ctx context.Context, req *pb.GetClaimRequest) (*pb.Claim, error) {
	if err := validateID(req.GetId(), "id"); err != nil {
		return nil, err
	}
	claim := &pb.Claim{}
	err := s.db.QueryRowContext(ctx,
		`SELECT id, item_id, claimant_name, claimant_email, status FROM claims WHERE id = $1`,
		req.GetId(),
	).Scan(&claim.Id, &claim.ItemId, &claim.ClaimantName, &claim.ClaimantEmail, &claim.Status)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, status.Errorf(codes.NotFound, "claim %d not found", req.GetId())
	}
	if err != nil {
		log.Printf("GetClaim DB error: %v", err)
		return nil, status.Error(codes.Internal, "failed to retrieve claim")
	}

	item, err := s.items.GetItem(ctx, claim.ItemId)
	switch {
	case err == nil:
		claim.ItemName = item.GetName()
	case status.Code(err) == codes.NotFound:
		claim.ItemName = "(item no longer exists)"
	default:
		log.Printf("GetClaim fallback: item service unavailable: %v", err)
		claim.ItemName = "(unavailable: item service is down)"
	}
	return claim, nil
}

func (s *server) UpdateClaim(ctx context.Context, req *pb.UpdateClaimRequest) (*pb.Claim, error) {
	if err := validateID(req.GetId(), "id"); err != nil {
		return nil, err
	}
	if err := validateClaimant(req.GetClaimantName(), req.GetClaimantEmail()); err != nil {
		return nil, err
	}
	if !validStatuses[req.GetStatus()] {
		return nil, status.Error(codes.InvalidArgument, "status must be one of PENDING, APPROVED, REJECTED")
	}

	claim := &pb.Claim{}
	err := s.db.QueryRowContext(ctx,
		`UPDATE claims SET claimant_name = $1, claimant_email = $2, status = $3
		 WHERE id = $4
		 RETURNING id, item_id, claimant_name, claimant_email, status`,
		strings.TrimSpace(req.GetClaimantName()), strings.TrimSpace(req.GetClaimantEmail()),
		req.GetStatus(), req.GetId(),
	).Scan(&claim.Id, &claim.ItemId, &claim.ClaimantName, &claim.ClaimantEmail, &claim.Status)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, status.Errorf(codes.NotFound, "claim %d not found", req.GetId())
	}
	if err != nil {
		log.Printf("UpdateClaim DB error: %v", err)
		return nil, status.Error(codes.Internal, "failed to update claim")
	}
	return claim, nil
}

func (s *server) DeleteClaim(ctx context.Context, req *pb.DeleteClaimRequest) (*pb.DeleteClaimResponse, error) {
	if err := validateID(req.GetId(), "id"); err != nil {
		return nil, err
	}
	res, err := s.db.ExecContext(ctx, `DELETE FROM claims WHERE id = $1`, req.GetId())
	if err != nil {
		log.Printf("DeleteClaim DB error: %v", err)
		return nil, status.Error(codes.Internal, "failed to delete claim")
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, status.Errorf(codes.NotFound, "claim %d not found", req.GetId())
	}
	return &pb.DeleteClaimResponse{Message: "claim deleted"}, nil
}

// ListClaims returns all claims from this service's own DB. It does not call
// Service B per row (to avoid N calls), so item_name is left empty here.
func (s *server) ListClaims(ctx context.Context, _ *pb.ListClaimsRequest) (*pb.ListClaimsResponse, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, item_id, claimant_name, claimant_email, status FROM claims ORDER BY id`)
	if err != nil {
		log.Printf("ListClaims DB error: %v", err)
		return nil, status.Error(codes.Internal, "failed to list claims")
	}
	defer rows.Close()

	resp := &pb.ListClaimsResponse{}
	for rows.Next() {
		c := &pb.Claim{}
		if err := rows.Scan(&c.Id, &c.ItemId, &c.ClaimantName, &c.ClaimantEmail, &c.Status); err != nil {
			log.Printf("ListClaims scan error: %v", err)
			return nil, status.Error(codes.Internal, "failed to list claims")
		}
		resp.Claims = append(resp.Claims, c)
	}
	if err := rows.Err(); err != nil {
		return nil, status.Error(codes.Internal, "failed to list claims")
	}
	return resp, nil
}
