package main

import (
	"database/sql"
	"fmt"
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
	maxPages:                  1000, //TODO: Get real max count from JSON
}

// Weapon info
type Weapon struct {
	Name  string
	Price float64
	URL   string
}

var urlbase = make(map[string]bool)
var err error

func init() {
	config.db, err = sql.Open("sqlite3", config.dbName)
	check(err)
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

func checkPrice() Weapon {

	// TODO
	var weapon Weapon
	return weapon
}

func main() {

	c := make(chan string, 10)
	///body := pageParser(1, c)
	for i := 1; i < config.maxPages; i += config.count {
		go pageParser(i, c)

		select {
		case pass := <-c:
			fmt.Println(0, i)
			contentConverter(pass)
		case pass1 := <-c:
			fmt.Println(1, i)
			contentConverter(pass1)
		case pass2 := <-c:
			fmt.Println(2, i)
			contentConverter(pass2)
		case pass3 := <-c:
			fmt.Println(3, i)
			contentConverter(pass3)

		}
		time.Sleep(time.Second)
	}

}
