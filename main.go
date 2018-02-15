package main

import (
	"database/sql"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
	_ "github.com/mattn/go-sqlite3"
)

var botconfig = struct {
	apiKey string
	chatID int64
}{
	apiKey: "CHANGE_ME",
	chatID: 0,
}

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
	discount                  int
	tAPIKey                   string
	tChat                     int64
}{
	dbName:                    "steam_database.sqlite",
	query:                     "",
	appid:                     "730",
	url:                       "http://steamcommunity.com",
	sortDir:                   "desc",
	count:                     100,
	searchDescriptions:        "0",
	sortColumn:                "price",
	category730ProPlayer:      "any",
	category730Weapon:         "any",
	category730TournamentTeam: "any",
	category730StickerCapsule: "any",
	maxPages:                  100,
	discount:                  20,
	tAPIKey:                   "PLEASE_CHANGE_ME",
	tChat:                     0, // PLEASE CHANGE ME
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

var err error

func check(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
func init() {
	config.db, err = sql.Open("sqlite3", config.dbName)
	check(err)

	if err = config.db.Ping(); err != nil {
		log.Fatal(err)
	}

	tAPIKeyF := flag.String("telegram-api-key", config.tAPIKey, "Telegram API KEY")
	tChatF := flag.Int64("telegram-channel", config.tChat, " Telegram Channel ID (default 0)")
	flag.Parse()

	config.tAPIKey = *tAPIKeyF
	config.tChat = *tChatF

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

	client := http.Client{Timeout: time.Duration(600) * time.Second}
	req, _ := http.NewRequest("GET", URL.String(), nil)
	resp, err := client.Do(req)
	check(err)
	if resp.Status != "200 OK" {
		log.Println("Scanner in cool down, wait 1 minute")
		time.Sleep(time.Minute)
		c <- "Ready"

	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	check(err)

	c <- string(body)

}

func contentConverter(page string) []Weapon {
	weaponPack := make([]Weapon, config.count)
	// Emergency out
	if !strings.Contains(page, "results_html") {
		return weaponPack
	}

	// Getting total_count
	totalCount := regexp.MustCompile(`total_count":\d*`).FindString(page)
	//fmt.Println(page)
	config.maxPages, err = strconv.Atoi(strings.Split(totalCount, ":")[1])
	check(err)
	// Main parsing
	page = regexp.MustCompile("\\\\").ReplaceAllString(page, "")
	output := strings.Split(page, "class")
	var weapon Weapon

	for _, value := range output {
		// Ugly ,fkn Steam.
		if strings.Contains(value, "market_listing_row_link") {
			urlArray := strings.Split(value, "\"")
			weapon.URL = urlArray[3]
		} else if strings.Contains(value, "\"normal_price") {
			priceString := regexp.MustCompile(`\d*[.]\d*`).FindString(value)
			weapon.Price, err = strconv.ParseFloat(priceString, 64)
			check(err)
		} else if strings.Contains(value, "market_listing_item_name\"") {
			nameArray := strings.Split(value, ">")
			nameArray = strings.Split(nameArray[1], "<")
			nameString := nameArray[0]
			weapon.Name = nameString
		} else if strings.Contains(value, "/a") {
			weaponPack = append(weaponPack, weapon)
		}
	}
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
		if (currentPrice.NewPrice * 100 / currentPrice.OldPrice) <= float64(100-config.discount) {
			sDiscount := strconv.Itoa(config.discount)
			sNewPrice := strconv.FormatFloat(currentPrice.NewPrice, 'f', -1, 64)
			sOldPrice := strconv.FormatFloat(currentPrice.OldPrice, 'f', -1, 64)
			message := "↓" + sDiscount + " > " + sNewPrice + "/" + sOldPrice + " " + weapon.Name + " " + weapon.URL
			tellMeBot(message)
			log.Println("↓", config.discount, ">", currentPrice.NewPrice, "/", currentPrice.OldPrice, weapon.Name, weapon.URL)

		}

	}

}

func tellMeBot(message string) {
	bot, err := tgbotapi.NewBotAPI(botconfig.apiKey)
	check(err)
	msg := tgbotapi.NewMessage(botconfig.chatID, "")
	msg.Text = message
	bot.Send(msg)

}

func main() {
	for {
		c := make(chan string, config.count)
		for i := 1; i < config.maxPages; i += config.count {
			go pageParser(i, c)

			select {
			case pass := <-c:
				fmt.Println("#gorutine:", 0, " #page:", i)
				processing(pass)
			case pass1 := <-c:
				fmt.Println("#gorutine:", 1, " #page:", i)
				processing(pass1)

			case pass2 := <-c:
				fmt.Println("#gorutine:", 2, " #page:", i)
				processing(pass2)
			}
			time.Sleep(time.Second)
		}
		time.Sleep(time.Second)
	}

}
