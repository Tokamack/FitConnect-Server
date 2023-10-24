package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	_ "github.com/lib/pq"
	"log"
	"net/http"
	"strconv"
	"strings"
)

type User struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

const (
	dbHost     = "localhost"
	dbPort     = "5432"
	dbUser     = "postgres"
	dbPassword = "toka"
	dbName     = "postgres"
)

func main() {
	http.HandleFunc("/clubs/get_list", handleClubsGetList)
	http.HandleFunc("/clubs/getFullInfo", handleClubsGetFullInfo)
	http.HandleFunc("/user/add", handleUserAdd)
	http.HandleFunc("/club/setFavouriteStatus", handleClubSetFavouriteStatus)

	log.Fatal(http.ListenAndServe(":8080", nil))
}

func getIdByToken(token string, db *sql.DB, w http.ResponseWriter) int {
	var id int
	err := db.QueryRow("SELECT id FROM users WHERE token = $1", token).Scan(&id)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "User not found", http.StatusForbidden)
		} else {
			http.Error(w, "Failed to retrieve user name", http.StatusInternalServerError)
			log.Println(err)
		}
	}
	return id
}

func handleUserAdd(w http.ResponseWriter, r *http.Request) {

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Подключение к базе данных PostgreSQL.
	dbInfo := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		dbHost, dbPort, dbUser, dbPassword, dbName)
	db, err := sql.Open("postgres", dbInfo)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// Разбор JSON из запроса.4
	var request struct {
		NickName string `json:"nick_name"`
		Token    string `json:"token"`
	}
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&request); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	query, err := db.Query("INSERT INTO users (nick_name, token, image) VALUES ($1, $2, $3);", request.NickName, request.Token, "")
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "No items found", http.StatusNotFound)
		} else {
			http.Error(w, "Failed to retrieve", http.StatusInternalServerError)
			log.Println(err)
		}
		return
	}
	defer query.Close()

	w.Header().Set("Content-Type", "application/json")
	response := struct {
		Status bool `json:"status"`
	}{
		Status: true,
	}

	json.NewEncoder(w).Encode(response)
	log.Println("Processed user add request SUCCESSFULLY " + request.Token)
}

func handleClubsGetList(w http.ResponseWriter, r *http.Request) {

	type ClubInfo struct {
		ID        int    `json:"id"`
		Name      string `json:"name"`
		ImageURLs string `json:"image_urls"`
		Location  struct {
			Latitude  float64 `json:"latitude"`
			Longitude float64 `json:"longitude"`
			Address   string  `json:"address"`
			City      string  `json:"city"`
			Metro     string  `json:"metro"`
		} `json:"location"`
		Score        float64 `json:"rating"`
		ReviewsCount int     `json:"rating_count"`
		Cost         string  `json:"cost"`
		Category     string  `json:"category"`
		IsFavorite   bool    `json:"isFavorite"`
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Подключение к базе данных
	dbInfo := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		dbHost, dbPort, dbUser, dbPassword, dbName)
	db, err := sql.Open("postgres", dbInfo)
	if err != nil {
		log.Fatal(err)
	}

	// Разбор JSON из запроса.
	var request struct {
		ClubsFilters struct {
			Favourites    bool  `json:"favourites"`
			Facilities    []int `json:"facilities"`
			Cost          []int `json:"cost"`
			ClubsCategory []int `json:"clubsCategory"`
			SortsType     int   `json:"sortsType"`
		} `json:"clubs_filters"`
		SearchBy  string `json:"search_by"`
		PageIndex int    `json:"page_index"`
		Token     string `json:"token"`
	}
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&request); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	const (
		ClubsCategory_Any      int = 0
		ClubsCategory_Gym      int = 1
		ClubsCategory_Box          = 2
		ClubsCategory_Swimming     = 3
	)

	const (
		SortsType_Az        int = 0
		SortsTyp_Za             = 1
		SortsTyp_Score          = 2
		SortsTyp_ScoreCount     = 3
	)

	//Get user id by token
	var id = getIdByToken(request.Token, db, w)
	defer db.Close()

	//SQL запрос
	var t string = `
		SELECT c.id, c.name, c.image_urls, c.latitude, c.longitude, c.address, c.city, c.metro, c.rating, c.reviewscount, c.expensiveness, c.category,
		CASE WHEN f.user_id IS NOT NULL THEN TRUE ELSE FALSE END AS is_favorite
		FROM clubs c
		LEFT JOIN favorites f ON c.id = f.object_id _FAVOURITES_
		WHERE 1=1
		_SEARCH_
		_CLUBS_CATEGORY_
		_COST_
		_FACILITIES_
		GROUP BY c.id, c.name, c.image_urls, c.latitude, c.longitude, c.address, c.city, c.metro, c.rating, c.reviewscount, c.expensiveness, is_favorite
		_SORT_
		LIMIT 20 OFFSET _PAGE_`

	//Заменяем _PAGE_ на номер страницы
	t = strings.Replace(t, "_PAGE_", strconv.Itoa(request.PageIndex*20), 1)

	//Добавляем поисковый запрос
	if request.SearchBy != "" {
		t = strings.Replace(t, "_SEARCH_", "AND c.name ILIKE '%"+request.SearchBy+"%' ", 1)
	} else {
		t = strings.Replace(t, "_SEARCH_", "", 1)
	}

	//Добавляем фильтр по категории
	if len(request.ClubsFilters.ClubsCategory) > 0 && request.ClubsFilters.ClubsCategory[0] != ClubsCategory_Any {
		var s string = ""
		for _, v := range request.ClubsFilters.ClubsCategory {
			s += strconv.Itoa(v) + ","
		}
		s = strings.TrimSuffix(s, ",")
		t = strings.Replace(t, "_CLUBS_CATEGORY_", " AND (ARRAY["+s+"] && c.category)", 1)

	} else {
		t = strings.Replace(t, "_CLUBS_CATEGORY_", "", 1)
	}

	//Добавляем фильтр по стоимости
	if len(request.ClubsFilters.Cost) > 0 && request.ClubsFilters.Cost[0] != 0 {
		var s string = ""
		for _, v := range request.ClubsFilters.Cost {
			s += strconv.Itoa(v) + ","
		}
		s = strings.TrimSuffix(s, ",")
		t = strings.Replace(t, "_COST_", " AND (ARRAY["+s+"] && c.costs)", 1)
	} else {
		t = strings.Replace(t, "_COST_", "", 1)
	}

	//Добавляем фильтр по избранномусд
	if request.ClubsFilters.Favourites {
		t = strings.Replace(t, "_FAVOURITES_", " AND f.user_id = "+strconv.Itoa(id)+" ", 1)
	} else {
		t = strings.Replace(t, "_FAVOURITES_", "", 1)
	}

	//Добавляем фильтр по удобствам
	if len(request.ClubsFilters.Facilities) > 0 && request.ClubsFilters.Facilities[0] != 0 {
		if len(request.ClubsFilters.Facilities) == 1 {
			t = strings.Replace(t, "_FACILITIES_", "AND "+strconv.Itoa(request.ClubsFilters.Facilities[0])+" = ANY(c.facilities)", 1)
		} else {
			var s string = ""
			for _, v := range request.ClubsFilters.Facilities {
				s += "c.facilities = " + strconv.Itoa(v) + " AND "
			}
			s = strings.TrimSuffix(s, "AND ")
			t = strings.Replace(t, "_FACILITIES_", "AND ("+s+") ", 1)
		}
	} else {
		t = strings.Replace(t, "_FACILITIES_", "", 1)
	}

	//Добавляем сортировку
	switch request.ClubsFilters.SortsType {
	case SortsType_Az:
		t = strings.Replace(t, "_SORT_", "ORDER BY c.name ASC", 1)
	case SortsTyp_Za:
		t = strings.Replace(t, "_SORT_", "ORDER BY c.name DESC", 1)
	case SortsTyp_Score:
		t = strings.Replace(t, "_SORT_", "ORDER BY c.rating DESC", 1)
	case SortsTyp_ScoreCount:
		t = strings.Replace(t, "_SORT_", "ORDER BY c.reviewscount DESC", 1)
	}

	//Actual query
	query, err := db.Query(t)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "No items found", http.StatusNotFound)
		} else {
			http.Error(w, "Failed to retrieve", http.StatusInternalServerError)
			log.Println(err)
		}
		return
	}
	defer query.Close()

	//Ответ

	w.Header().Set("Content-Type", "application/json")

	var results []ClubInfo
	// Перебираем строки результата
	for query.Next() {
		var result ClubInfo
		// Считываем значения в переменные
		err := query.Scan(&result.ID, &result.Name, &result.ImageURLs, &result.Location.Latitude, &result.Location.Longitude, &result.Location.Address, &result.Location.City, &result.Location.Metro, &result.Score, &result.ReviewsCount, &result.Cost, &result.Category, &result.IsFavorite)
		if err != nil {
			log.Fatal(err)
		}
		results = append(results, result)
	}

	// Проверяем наличие ошибок после перебора результатов
	if err := query.Err(); err != nil {
		log.Fatal(err)
	}

	// Преобразуем результат в JSON и выводим
	jsonResult, err := json.Marshal(results)
	if err != nil {
		log.Fatal(err)
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(jsonResult)
	log.Println("\nProcessed clubs list request SUCCESSFULLY " + request.Token)

}

func handleClubsGetFullInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Подключение к базе данных
	dbInfo := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		dbHost, dbPort, dbUser, dbPassword, dbName)
	db, err := sql.Open("postgres", dbInfo)
	if err != nil {
		log.Fatal(err)
	}

	// Разбор JSON из запроса.
	var request struct {
		Id    int    `json:"club_id"`
		Token string `json:"token"`
	}
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&request); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	//Get user id by token
	var id = getIdByToken(request.Token, db, w)
	defer db.Close()

	//SQL запрос
	var t string = `
		SELECT c.id, c.name, c.image_urls, c.latitude, c.longitude, c.address, c.city, c.metro, c.rating, c.admin_id, c.admin_name, c.admin_phone, c.description, c.reviewscount, c.expensiveness, c.category,  c.facilities,
       	CASE WHEN f.user_id = $1 THEN TRUE ELSE FALSE END AS is_favorite
		FROM clubs c
        LEFT JOIN favorites f ON c.id = f.object_id
		WHERE c.id = $2
		GROUP BY c.id, c.name, c.image_urls, c.latitude, c.longitude, c.address, c.city, c.metro, c.rating, c.reviewscount, c.expensiveness, is_favorite
		`

	//Actual query
	responce := struct {
		ID        int    `json:"id"`
		Name      string `json:"name"`
		ImageURLs string `json:"image_urls"`
		Location  struct {
			Latitude  float64 `json:"latitude"`
			Longitude float64 `json:"longitude"`
			Address   string  `json:"address"`
			City      string  `json:"city"`
			Metro     string  `json:"metro"`
		} `json:"location"`
		Score float64 `json:"rating"`
		Admin struct {
			AdminId    int    `json:"id"`
			AdminName  string `json:"name"`
			AdminPhone string `json:"phone"`
		} `json:"admin"`
		Description  string `json:"description"`
		ReviewsCount int    `json:"rating_count"`
		Cost         int    `json:"cost"`
		Category     string `json:"category"`
		Facilities   string `json:"facilities"`
		IsFavorite   bool   `json:"isFavorite"`
	}{}
	err = db.QueryRow(t, id, request.Id).Scan(&responce.ID, &responce.Name, &responce.ImageURLs, &responce.Location.Latitude, &responce.Location.Longitude, &responce.Location.Address, &responce.Location.City, &responce.Location.Metro, &responce.Score, &responce.Admin.AdminId, &responce.Admin.AdminName, &responce.Admin.AdminPhone, &responce.Description, &responce.ReviewsCount, &responce.Cost, &responce.Category, &responce.Facilities, &responce.IsFavorite)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "No items found", http.StatusNotFound)
		} else {
			http.Error(w, "Failed to retrieve", http.StatusInternalServerError)
			log.Println(err)
		}
		return
	}

	//Ответ
	w.Header().Set("Content-Type", "application/json")

	// Преобразуем результат в JSON и выводим
	jsonResult, err := json.Marshal(responce)
	if err != nil {
		log.Fatal(err)
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(jsonResult)

	log.Println("\nProcessed clubs list request SUCCESSFULLY " + request.Token)

}

func handleClubSetFavouriteStatus(w http.ResponseWriter, r *http.Request) {

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Подключение к базе данных PostgreSQL.
	dbInfo := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		dbHost, dbPort, dbUser, dbPassword, dbName)
	db, err := sql.Open("postgres", dbInfo)
	if err != nil {
		log.Fatal(err)
	}

	// Разбор JSON из запроса.
	var request struct {
		ClubId int    `json:"club_id"`
		Status bool   `json:"status"`
		Token  string `json:"token"`
	}
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&request); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	//Get user id by token
	var id = getIdByToken(request.Token, db, w)
	defer db.Close()

	var t string
	if request.Status {
		t = "INSERT INTO favorites (user_id, object_id, object_type) VALUES ($1, $2, 'club');"
	} else {
		t = "DELETE FROM favorites WHERE user_id = $1 AND object_id = $2;"
	}

	query, err := db.Query(t, id, request.ClubId)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "No items found", http.StatusNotFound)
		} else {
			http.Error(w, "Failed to retrieve", http.StatusInternalServerError)
			log.Println(err)
		}
		return
	}
	defer query.Close()

	w.Header().Set("Content-Type", "application/json")
	response := struct {
		NewStatus bool `json:"new_status"`
	}{
		NewStatus: !request.Status,
	}

	json.NewEncoder(w).Encode(response)

	log.Println("Processed club set favourite status request SUCCESSFULLY " + request.Token)
}
