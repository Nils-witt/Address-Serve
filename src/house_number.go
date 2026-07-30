package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
)

type HouseNumber struct {
	ID             uuid.UUID `json:"id"`
	StreetID       uuid.UUID `json:"streetId"`
	Number         int       `json:"number"`
	NumberAddition *string   `json:"numberAddition,omitempty"`
	Latitude       float64   `json:"latitude"`
	Longitude      float64   `json:"longitude"`
}

func (h HouseNumber) validate() error {
	if h.StreetID == uuid.Nil {
		return errors.New("streetId is required")
	}
	if h.Number <= 0 {
		return errors.New("number must be positive")
	}
	if h.Latitude < -90 || h.Latitude > 90 {
		return errors.New("latitude must be between -90 and 90")
	}
	if h.Longitude < -180 || h.Longitude > 180 {
		return errors.New("longitude must be between -180 and 180")
	}
	return nil
}

var errStreetNotFound = errors.New("referenced street does not exist")

func isForeignKeyViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23503"
}

type houseNumberStore struct {
	db *sql.DB
}

func (st *houseNumberStore) create(h HouseNumber) (HouseNumber, error) {
	err := st.db.QueryRow(
		`INSERT INTO house_numbers (street_id, number, number_addition, latitude, longitude)
		 VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		h.StreetID, h.Number, h.NumberAddition, h.Latitude, h.Longitude,
	).Scan(&h.ID)
	if isForeignKeyViolation(err) {
		return HouseNumber{}, errStreetNotFound
	}
	return h, err
}

type houseNumberFilter struct {
	StreetID *uuid.UUID
	Number   *int
}

func (st *houseNumberStore) list(filter houseNumberFilter) ([]HouseNumber, error) {
	query := `SELECT id, street_id, number, number_addition, latitude, longitude FROM house_numbers`
	var conditions []string
	var args []any

	if filter.StreetID != nil {
		args = append(args, *filter.StreetID)
		conditions = append(conditions, fmt.Sprintf("street_id = $%d", len(args)))
	}
	if filter.Number != nil {
		args = append(args, *filter.Number)
		conditions = append(conditions, fmt.Sprintf("number = $%d", len(args)))
	}
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += ` ORDER BY id`

	rows, err := st.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	houseNumbers := []HouseNumber{}
	for rows.Next() {
		var h HouseNumber
		if err := rows.Scan(&h.ID, &h.StreetID, &h.Number, &h.NumberAddition, &h.Latitude, &h.Longitude); err != nil {
			return nil, err
		}
		houseNumbers = append(houseNumbers, h)
	}
	return houseNumbers, rows.Err()
}

func (st *houseNumberStore) get(id uuid.UUID) (HouseNumber, error) {
	var h HouseNumber
	err := st.db.QueryRow(
		`SELECT id, street_id, number, number_addition, latitude, longitude FROM house_numbers WHERE id = $1`, id,
	).Scan(&h.ID, &h.StreetID, &h.Number, &h.NumberAddition, &h.Latitude, &h.Longitude)
	return h, err
}

func (st *houseNumberStore) update(id uuid.UUID, h HouseNumber) (HouseNumber, error) {
	h.ID = id
	res, err := st.db.Exec(
		`UPDATE house_numbers SET street_id=$1, number=$2, number_addition=$3, latitude=$4, longitude=$5 WHERE id=$6`,
		h.StreetID, h.Number, h.NumberAddition, h.Latitude, h.Longitude, id,
	)
	if isForeignKeyViolation(err) {
		return HouseNumber{}, errStreetNotFound
	}
	if err != nil {
		return HouseNumber{}, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return HouseNumber{}, err
	}
	if n == 0 {
		return HouseNumber{}, sql.ErrNoRows
	}
	return h, nil
}

func (st *houseNumberStore) delete(id uuid.UUID) error {
	res, err := st.db.Exec(`DELETE FROM house_numbers WHERE id = $1`, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func houseNumberIDFromRequest(r *http.Request) (uuid.UUID, error) {
	return uuid.Parse(r.PathValue("id"))
}

func registerHouseNumberRoutes(mux *http.ServeMux, store *houseNumberStore) {
	mux.HandleFunc("POST /api/house-numbers", func(w http.ResponseWriter, r *http.Request) {
		var h HouseNumber
		if err := json.NewDecoder(r.Body).Decode(&h); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		if err := h.validate(); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		created, err := store.create(h)
		if errors.Is(err, errStreetNotFound) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, created)
	})

	mux.HandleFunc("GET /api/house-numbers", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()

		var filter houseNumberFilter
		if v := query.Get("streetId"); v != "" {
			id, err := uuid.Parse(v)
			if err != nil {
				writeError(w, http.StatusBadRequest, "invalid streetId")
				return
			}
			filter.StreetID = &id
		}
		if v := query.Get("number"); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil {
				writeError(w, http.StatusBadRequest, "invalid number")
				return
			}
			filter.Number = &n
		}

		houseNumbers, err := store.list(filter)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, houseNumbers)
	})

	mux.HandleFunc("GET /api/house-numbers/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := houseNumberIDFromRequest(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid id")
			return
		}
		h, err := store.get(id)
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "house number not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, h)
	})

	mux.HandleFunc("PUT /api/house-numbers/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := houseNumberIDFromRequest(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid id")
			return
		}
		var h HouseNumber
		if err := json.NewDecoder(r.Body).Decode(&h); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		if err := h.validate(); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		updated, err := store.update(id, h)
		if errors.Is(err, errStreetNotFound) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "house number not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, updated)
	})

	mux.HandleFunc("DELETE /api/house-numbers/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := houseNumberIDFromRequest(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid id")
			return
		}
		err = store.delete(id)
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "house number not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}
