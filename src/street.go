package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
)

type Street struct {
	ID        uuid.UUID `json:"id"`
	City      string    `json:"city"`
	District  string    `json:"district"`
	Name      string    `json:"name"`
	Latitude  float64   `json:"latitude"`
	Longitude float64   `json:"longitude"`
}

func (s Street) validate() error {
	if s.City == "" || s.District == "" || s.Name == "" {
		return errors.New("city, district and name are required")
	}
	if s.Latitude < -90 || s.Latitude > 90 {
		return errors.New("latitude must be between -90 and 90")
	}
	if s.Longitude < -180 || s.Longitude > 180 {
		return errors.New("longitude must be between -180 and 180")
	}
	return nil
}

var errStreetAlreadyExists = errors.New("a street with this name, district and city already exists")

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

type streetStore struct {
	db *sql.DB
}

func (st *streetStore) create(s Street) (Street, error) {
	err := st.db.QueryRow(
		`INSERT INTO streets (city, district, name, latitude, longitude)
		 VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		s.City, s.District, s.Name, s.Latitude, s.Longitude,
	).Scan(&s.ID)
	if isUniqueViolation(err) {
		return Street{}, errStreetAlreadyExists
	}
	return s, err
}

type streetFilter struct {
	City     string
	District string
	Name     string
}

func (st *streetStore) list(filter streetFilter) ([]Street, error) {
	query := `SELECT id, city, district, name, latitude, longitude FROM streets`
	var conditions []string
	var args []any

	if filter.City != "" {
		args = append(args, filter.City)
		conditions = append(conditions, fmt.Sprintf("city ILIKE $%d", len(args)))
	}
	if filter.District != "" {
		args = append(args, filter.District)
		conditions = append(conditions, fmt.Sprintf("district ILIKE $%d", len(args)))
	}
	if filter.Name != "" {
		args = append(args, "%"+filter.Name+"%")
		conditions = append(conditions, fmt.Sprintf("name ILIKE $%d", len(args)))
	}
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY id"

	rows, err := st.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	streets := []Street{}
	for rows.Next() {
		var s Street
		if err := rows.Scan(&s.ID, &s.City, &s.District, &s.Name, &s.Latitude, &s.Longitude); err != nil {
			return nil, err
		}
		streets = append(streets, s)
	}
	return streets, rows.Err()
}

func (st *streetStore) listCities(name string) ([]string, error) {
	query := `SELECT DISTINCT city FROM streets`
	var args []any
	if name != "" {
		args = append(args, "%"+name+"%")
		query += fmt.Sprintf(" WHERE city ILIKE $%d", len(args))
	}
	query += " ORDER BY city"
	return queryStrings(st.db, query, args...)
}

type districtFilter struct {
	Name string
	City string
}

type District struct {
	Name string `json:"name"`
	City string `json:"city"`
}

func (st *streetStore) listDistricts(filter districtFilter) ([]District, error) {
	query := `SELECT DISTINCT district, city FROM streets`
	var conditions []string
	var args []any

	if filter.Name != "" {
		args = append(args, "%"+filter.Name+"%")
		conditions = append(conditions, fmt.Sprintf("district ILIKE $%d", len(args)))
	}
	if filter.City != "" {
		args = append(args, filter.City)
		conditions = append(conditions, fmt.Sprintf("city ILIKE $%d", len(args)))
	}
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY district, city"

	rows, err := st.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	districts := []District{}
	for rows.Next() {
		var d District
		if err := rows.Scan(&d.Name, &d.City); err != nil {
			return nil, err
		}
		districts = append(districts, d)
	}
	return districts, rows.Err()
}

func queryStrings(db *sql.DB, query string, args ...any) ([]string, error) {
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	values := []string{}
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		values = append(values, v)
	}
	return values, rows.Err()
}

func (st *streetStore) get(id uuid.UUID) (Street, error) {
	var s Street
	err := st.db.QueryRow(
		`SELECT id, city, district, name, latitude, longitude FROM streets WHERE id = $1`, id,
	).Scan(&s.ID, &s.City, &s.District, &s.Name, &s.Latitude, &s.Longitude)
	return s, err
}

func (st *streetStore) update(id uuid.UUID, s Street) (Street, error) {
	s.ID = id
	res, err := st.db.Exec(
		`UPDATE streets SET city=$1, district=$2, name=$3, latitude=$4, longitude=$5 WHERE id=$6`,
		s.City, s.District, s.Name, s.Latitude, s.Longitude, id,
	)
	if isUniqueViolation(err) {
		return Street{}, errStreetAlreadyExists
	}
	if err != nil {
		return Street{}, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return Street{}, err
	}
	if n == 0 {
		return Street{}, sql.ErrNoRows
	}
	return s, nil
}

func (st *streetStore) delete(id uuid.UUID) error {
	res, err := st.db.Exec(`DELETE FROM streets WHERE id = $1`, id)
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

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func streetIDFromRequest(r *http.Request) (uuid.UUID, error) {
	return uuid.Parse(r.PathValue("id"))
}

func registerStreetRoutes(mux *http.ServeMux, store *streetStore) {
	mux.HandleFunc("POST /api/streets", func(w http.ResponseWriter, r *http.Request) {
		var s Street
		if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		if err := s.validate(); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		created, err := store.create(s)
		if errors.Is(err, errStreetAlreadyExists) {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, created)
	})

	mux.HandleFunc("GET /api/streets", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		filter := streetFilter{
			City:     query.Get("city"),
			District: query.Get("district"),
			Name:     query.Get("name"),
		}
		streets, err := store.list(filter)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, streets)
	})

	mux.HandleFunc("GET /api/cities", func(w http.ResponseWriter, r *http.Request) {
		cities, err := store.listCities(r.URL.Query().Get("name"))
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, cities)
	})

	mux.HandleFunc("GET /api/districts", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		filter := districtFilter{
			Name: query.Get("name"),
			City: query.Get("city"),
		}
		districts, err := store.listDistricts(filter)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, districts)
	})

	mux.HandleFunc("GET /api/streets/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := streetIDFromRequest(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid id")
			return
		}
		s, err := store.get(id)
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "street not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, s)
	})

	mux.HandleFunc("PUT /api/streets/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := streetIDFromRequest(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid id")
			return
		}
		var s Street
		if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		if err := s.validate(); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		updated, err := store.update(id, s)
		if errors.Is(err, errStreetAlreadyExists) {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "street not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, updated)
	})

	mux.HandleFunc("DELETE /api/streets/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := streetIDFromRequest(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid id")
			return
		}
		err = store.delete(id)
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "street not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}
