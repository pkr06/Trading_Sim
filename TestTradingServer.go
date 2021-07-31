package main

import (
	"errors"
	"fmt"
	"time"
	"log"
	"math"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"strings"
	"github.com/bakins/net-http-recover"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/gorilla/rpc"
	"github.com/gorilla/rpc/json"
	"github.com/justinas/alice"
	"github.com/tiloso/googlefinance"
)

type Quote struct {
  Date                    time.Time
  Open, High, Low, Close  float64
  Volume                  uint32
}

type PurchaseRequest struct {
	Budget                   float32
	StockSymbolAndPercentage string
}

type PurchaseResponse struct {
	TradeId         int
	Stocks           []string
	UninvestedAmount float32
}

type CheckResponse struct {
	Stocks           []string
	CurrentMarketValue float32
	UninvestedAmount float32

}

type CheckRequest struct {
	TradeId string
}

type StockAccounts struct {
	stockPortfolio map[int](*Portfolio)
}

type Portfolio struct {
	stocks           map[string](*Share)
	uninvestedAmount float32
}

type Share struct {
	shareNum    int
	boughtPrice float32
}


var st StockAccounts

var tradeId int

func (st *StockAccounts) Buy(httpRq *http.Request, rq *PurchaseRequest, rsp *PurchaseResponse) error {

	tradeId++
	rsp.TradeId = tradeId

	if st.stockPortfolio == nil {

		st.stockPortfolio = make(map[int](*Portfolio))

		st.stockPortfolio[tradeId] = new(Portfolio)
		st.stockPortfolio[tradeId].stocks = make(map[string]*Share)

	}

	symbolAndPercentages := strings.Split(rq.StockSymbolAndPercentage, ",")
	newBudget := float32(rq.Budget)

	var amtSpent float32

	for _, stk := range symbolAndPercentages {

		splited := strings.Split(stk, ":")
		stkQuote := splited[0]
		percentage := splited[1]
		strPercentage := strings.TrimSuffix(percentage, "%")
		floatPercentage64, _ := strconv.ParseFloat(strPercentage, 32)
		floatPercentage := float32(floatPercentage64 / 100.00)
		currentPrice := checkQuote(stkQuote)

		shares := int(math.Floor(float64(newBudget * floatPercentage / currentPrice)))
		sharesFloat := float32(shares)
		amtSpent += sharesFloat * currentPrice

		if _, ok := st.stockPortfolio[tradeId]; !ok {

			newPortfolio := new(Portfolio)
			newPortfolio.stocks = make(map[string]*Share)
			st.stockPortfolio[tradeId] = newPortfolio
		}
		if _, ok := st.stockPortfolio[tradeId].stocks[stkQuote]; !ok {

			newShare := new(Share)
			newShare.boughtPrice = currentPrice
			newShare.shareNum = shares
			st.stockPortfolio[tradeId].stocks[stkQuote] = newShare
		} else {

			total := float32(sharesFloat*currentPrice) + float32(st.stockPortfolio[tradeId].stocks[stkQuote].shareNum)*st.stockPortfolio[tradeId].stocks[stkQuote].boughtPrice
			st.stockPortfolio[tradeId].stocks[stkQuote].boughtPrice = total / float32(shares+st.stockPortfolio[tradeId].stocks[stkQuote].shareNum)
			st.stockPortfolio[tradeId].stocks[stkQuote].shareNum += shares
		}

		stockBought := stkQuote + ":" + strconv.Itoa(shares) + ":$" + strconv.FormatFloat(float64(currentPrice), 'f', 2, 32)

		rsp.Stocks = append(rsp.Stocks, stockBought)
	}

	amtLeftOver := newBudget - amtSpent
	rsp.UninvestedAmount = amtLeftOver
	st.stockPortfolio[tradeId].uninvestedAmount += amtLeftOver

	return nil
}

func (st *StockAccounts) Check(httpRq *http.Request, checkRq *CheckRequest, checkResp *CheckResponse) error {

	if st.stockPortfolio == nil {
		return errors.New("No account set up yet.")
	}

	tradeId64, err := strconv.ParseInt(checkRq.TradeId, 10, 64)

	if err != nil {
		return errors.New("Invalid Trade ID. ")
	}
	tradeId := int(tradeId64)

	if pocket, ok := st.stockPortfolio[tradeId]; ok {

		var currentMarketVal float32
		for stockquote, sh := range pocket.stocks {

			currentPrice := checkQuote(stockquote)

			var str string
			if sh.boughtPrice < currentPrice {
				str = "+$" + strconv.FormatFloat(float64(currentPrice), 'f', 2, 32)
			} else if sh.boughtPrice > currentPrice {
				str = "-$" + strconv.FormatFloat(float64(currentPrice), 'f', 2, 32)
			} else {
				str = "$" + strconv.FormatFloat(float64(currentPrice), 'f', 2, 32)
			}

			entry := stockquote + ":" + strconv.Itoa(sh.shareNum) + ":" + str
			checkResp.Stocks = append(checkResp.Stocks, entry)
			currentMarketVal += float32(sh.shareNum) * currentPrice
		}

		checkResp.UninvestedAmount = pocket.uninvestedAmount

		checkResp.CurrentMarketValue = currentMarketVal
	} else {
		return errors.New("This trade ID doesn't exists")
	}

	return nil
}

func main() {

	var st = (new(StockAccounts))

	tradeId = rand.Intn(99999) + 1

	router := mux.NewRouter()
	server := rpc.NewServer()
	server.RegisterCodec(json.NewCodec(), "application/json")
	server.RegisterService(st, "")

	chain := alice.New(
		func(h http.Handler) http.Handler {
			return handlers.CombinedLoggingHandler(os.Stdout, h)
		},
		handlers.CompressHandler,
		func(h http.Handler) http.Handler {
			return recovery.Handler(os.Stderr, h, true)
		})

	router.Handle("/rpc", chain.Then(server))
	log.Fatal(http.ListenAndServe(":8070", server))

}

func checkError(err error) {
	if err != nil {
		fmt.Println(err)
	}
}

func checkQuote(stockName string) float32 {

	var qs []Quote
	t := time.Now()
	t1 := t.Add(time.Duration(1) * time.Hour * 24)

	if err := googlefinance.Range(t, t1).Key("NASDAQ:" + stockName ).Get(&qs); err != nil {
		fmt.Printf("err: %v\n", err)
	}

	if len(qs) > 0 {
		return (float32( qs[0].Close))
	} else{
		return 0
	}
}
