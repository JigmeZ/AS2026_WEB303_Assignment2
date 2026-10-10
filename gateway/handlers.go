package main

import (
	"context"
	"net/http"
	"time"

	claimpb "lostfound/gen/claimpb"
	itempb "lostfound/gen/itempb"
)

// Gateway holds the gRPC clients used by the REST handlers.
type Gateway struct {
	items   itempb.ItemServiceClient
	claims  claimpb.ClaimServiceClient
	timeout time.Duration
}

// JSON request bodies. Field names are what Postman sends.
type itemBody struct {
	Name          string `json:"name"`
	Description   string `json:"description"`
	LocationFound string `json:"location_found"`
	Status        string `json:"status"`
}

type claimBody struct {
	ItemID        int64  `json:"item_id"`
	ClaimantName  string `json:"claimant_name"`
	ClaimantEmail string `json:"claimant_email"`
	Status        string `json:"status"`
}

func (g *Gateway) ctx(r *http.Request) (context.Context, context.CancelFunc) {
	return context.WithTimeout(r.Context(), g.timeout)
}

// ======================= ITEMS =======================

func (g *Gateway) createItem(w http.ResponseWriter, r *http.Request) {
	var b itemBody
	if err := decodeBody(w, r, &b); err != nil {
		badRequest(w, "invalid JSON body: "+err.Error())
		return
	}
	ctx, cancel := g.ctx(r)
	defer cancel()
	item, err := g.items.CreateItem(ctx, &itempb.CreateItemRequest{
		Name: b.Name, Description: b.Description, LocationFound: b.LocationFound,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, item) // 201
}

func (g *Gateway) listItems(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := g.ctx(r)
	defer cancel()
	resp, err := g.items.ListItems(ctx, &itempb.ListItemsRequest{})
	if err != nil {
		writeError(w, err)
		return
	}
	items := resp.GetItems()
	if items == nil {
		items = []*itempb.Item{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"items": items})
}

func (g *Gateway) getItem(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	ctx, cancel := g.ctx(r)
	defer cancel()
	item, err := g.items.GetItem(ctx, &itempb.GetItemRequest{Id: id})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (g *Gateway) updateItem(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	var b itemBody
	if err := decodeBody(w, r, &b); err != nil {
		badRequest(w, "invalid JSON body: "+err.Error())
		return
	}
	ctx, cancel := g.ctx(r)
	defer cancel()
	item, err := g.items.UpdateItem(ctx, &itempb.UpdateItemRequest{
		Id: id, Name: b.Name, Description: b.Description,
		LocationFound: b.LocationFound, Status: b.Status,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (g *Gateway) deleteItem(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	ctx, cancel := g.ctx(r)
	defer cancel()
	if _, err := g.items.DeleteItem(ctx, &itempb.DeleteItemRequest{Id: id}); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent) // 204
}

// ======================= CLAIMS =======================

func (g *Gateway) createClaim(w http.ResponseWriter, r *http.Request) {
	var b claimBody
	if err := decodeBody(w, r, &b); err != nil {
		badRequest(w, "invalid JSON body: "+err.Error())
		return
	}
	ctx, cancel := g.ctx(r)
	defer cancel()
	claim, err := g.claims.CreateClaim(ctx, &claimpb.CreateClaimRequest{
		ItemId: b.ItemID, ClaimantName: b.ClaimantName, ClaimantEmail: b.ClaimantEmail,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, claim) // 201
}

func (g *Gateway) listClaims(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := g.ctx(r)
	defer cancel()
	resp, err := g.claims.ListClaims(ctx, &claimpb.ListClaimsRequest{})
	if err != nil {
		writeError(w, err)
		return
	}
	claims := resp.GetClaims()
	if claims == nil {
		claims = []*claimpb.Claim{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"claims": claims})
}

func (g *Gateway) getClaim(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	ctx, cancel := g.ctx(r)
	defer cancel()
	claim, err := g.claims.GetClaim(ctx, &claimpb.GetClaimRequest{Id: id})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, claim)
}

func (g *Gateway) updateClaim(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	var b claimBody
	if err := decodeBody(w, r, &b); err != nil {
		badRequest(w, "invalid JSON body: "+err.Error())
		return
	}
	ctx, cancel := g.ctx(r)
	defer cancel()
	claim, err := g.claims.UpdateClaim(ctx, &claimpb.UpdateClaimRequest{
		Id: id, ClaimantName: b.ClaimantName, ClaimantEmail: b.ClaimantEmail, Status: b.Status,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, claim)
}

func (g *Gateway) deleteClaim(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	ctx, cancel := g.ctx(r)
	defer cancel()
	if _, err := g.claims.DeleteClaim(ctx, &claimpb.DeleteClaimRequest{Id: id}); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent) // 204
}
