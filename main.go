package main

import (
	"database/sql"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

var config = struct {
	db                        *sql.DB
	dbName                    string
	query                     string
	appid                     string
	url                       string
	sortDir                   string
	count                     int
	searchDescriptions        string
	sortColumn                string
	category730ProPlayer      string
	category730Weapon         string
	category730TournamentTeam string
	category730StickerCapsule string
	maxPages                  int
}{
	dbName:                    "steam_database.sqlite",
	query:                     "",
	appid:                     "730",
	url:                       "http://steamcommunity.com",
	sortDir:                   "desc",
	count:                     10,
	searchDescriptions:        "0",
	sortColumn:                "price",
	category730ProPlayer:      "any",
	category730Weapon:         "any",
	category730TournamentTeam: "any",
	category730StickerCapsule: "any",
	maxPages:                  100, //TODO: Get real max count from JSON
}

// Weapon info
type Weapon struct {
	Name  string
	Price float64
	URL   string
}

// Price from DB
type Price struct {
	Name     string
	NewPrice float64
	OldPrice float64
	MinPrice float64
	MaxPrice float64
}

var urlbase = make(map[string]bool)
var err error

func init() {
	config.db, err = sql.Open("sqlite3", config.dbName)
	check(err)

	if err = config.db.Ping(); err != nil {
		log.Fatal(err)
	}
}

func check(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func pageParser(start int, c chan string) {

	var URL *url.URL

	URL, err := URL.Parse(config.url + "/market/search/render/")
	check(err)

	parameters := url.Values{}

	parameters.Add("query", config.query)
	parameters.Add("appid", config.appid)
	parameters.Add("start", strconv.Itoa(start))

	parameters.Add("count", strconv.Itoa(config.count))
	parameters.Add("category_730_ProPlayer[]", config.category730ProPlayer)
	parameters.Add("category_730_StickerCapsule[]", config.category730StickerCapsule)
	parameters.Add("category_730_TournamentTeam[]", config.category730TournamentTeam)
	parameters.Add("category_730_Weapon[]", config.category730Weapon)
	URL.RawQuery = parameters.Encode()

	client := http.Client{Timeout: time.Duration(60) * time.Second}
	req, _ := http.NewRequest("GET", URL.String(), nil)
	resp, err := client.Do(req)
	check(err)

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	check(err)
	c <- string(body)
	//return string(body)

}

func contentConverter(page string) []Weapon {

	weaponPack := make([]Weapon, config.count)

	page = regexp.MustCompile("\\\\").ReplaceAllString(page, "")
	output := strings.Split(page, "class")
	var weapon Weapon

	for _, value := range output {
		// Ugly parsing. Fkn Steam.
		if strings.Contains(value, "market_listing_row_link") {
			urlArray := strings.Split(value, "\"")
			weapon.URL = urlArray[3]
			//fmt.Println(weapon.URL)
		} else if strings.Contains(value, "\"normal_price") {
			priceString := regexp.MustCompile(`\d*[.]\d*`).FindString(value)
			weapon.Price, err = strconv.ParseFloat(priceString, 64)
			check(err)
			//fmt.Println(weapon.Price)
		} else if strings.Contains(value, "market_listing_item_name\"") {
			//fmt.Println(value)
			nameArray := strings.Split(value, ">")
			nameArray = strings.Split(nameArray[1], "<")
			nameString := nameArray[0]
			weapon.Name = nameString
			//fmt.Println(weapon.Name)
		} else if strings.Contains(value, "/a") {
			weaponPack = append(weaponPack, weapon)
		}
	}
	//log.Println(weaponPack)
	return weaponPack

}

func setPrice(newPrice Price) {
	tx, err := config.db.Begin()
	check(err)

	sqlQueryPrice := "insert or replace into items (id_items, new_price, old_price, min_price, max_price) values (?, ?, ?, ?, ?)"

	insertPriceState, err := tx.Prepare(sqlQueryPrice)
	check(err)
	defer insertPriceState.Close()
	_, err = insertPriceState.Exec(newPrice.Name, newPrice.NewPrice, newPrice.OldPrice, newPrice.MinPrice, newPrice.MaxPrice)
	check(err)

	tx.Commit()

}

func getPrice(weapon Weapon) Price {

	var price Price
	sqlSelectQuery := "select new_price, old_price, min_price, max_price from items where id_items=?"
	query, err := config.db.Prepare(sqlSelectQuery)
	check(err)
	defer query.Close()

	var newPrice float64
	var oldPrice float64
	var minPrice float64
	var maxPrice float64

	err = query.QueryRow(weapon.Name).Scan(&newPrice, &oldPrice, &minPrice, &maxPrice)
	if err != nil {

		newPrice = 0
		oldPrice = 0
		minPrice = 0
		maxPrice = 0
	}
	price.Name = weapon.Name
	price.NewPrice = newPrice
	price.OldPrice = oldPrice
	price.MinPrice = minPrice
	price.MaxPrice = maxPrice

	//log.Println(price)
	return price

}

func processing(page string) {

	weaponPack := contentConverter(page)

	for _, weapon := range weaponPack {
		if weapon.Name == "" {
			continue
		}
		currentPrice := getPrice(weapon)
		var newPrice Price
		if currentPrice.OldPrice == 0 {
			newPrice.Name = weapon.Name
			newPrice.NewPrice = weapon.Price
			newPrice.OldPrice = weapon.Price
			newPrice.MinPrice = weapon.Price
			newPrice.MaxPrice = weapon.Price
			setPrice(newPrice)
		} else {
			currentPrice.OldPrice = currentPrice.NewPrice
			currentPrice.NewPrice = weapon.Price
			if weapon.Price < currentPrice.MinPrice {
				currentPrice.MinPrice = weapon.Price
			}
			if weapon.Price > currentPrice.MaxPrice {
				currentPrice.MaxPrice = weapon.Price
			}
			setPrice(currentPrice)

		}
		//Main trigger
		if currentPrice.NewPrice < currentPrice.OldPrice {
			log.Println("!!! Difference...", currentPrice.NewPrice, " and ", currentPrice.OldPrice)

		}
		//log.Println("Processing..." + weapon.Name)

	}

}

func main() {
	for {
		c := make(chan string, 10)
		for i := 1; i < config.maxPages; i += config.count {
			go pageParser(i, c)

			select {
			case pass := <-c:
				//fmt.Println(0, i)
				processing(pass)
			case pass1 := <-c:
				//fmt.Println(1, i)
				processing(pass1)
			}
			time.Sleep(time.Second)
		}
		time.Sleep(time.Second * 300)
	}

}
