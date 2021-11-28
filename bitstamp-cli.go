package main

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/georlav/bitstamp"
	"github.com/georlav/bitstamp-cli/internal/activepair"
	ui "github.com/gizak/termui/v3"
	"github.com/gizak/termui/v3/widgets"
)

func main() {
	var (
		bitClient                      = bitstamp.NewHTTPAPI()
		currencyListData               = []string{"1. ALL", "2. BTC", "3. EUR", "4. GBP", "5. USD"}
		pairMap                        = make(map[string]bitstamp.Pair)
		pairFullList, pairFullListData = getPairs(bitstamp.GetAllPairs())
		_, pairEuroListData            = getPairs(bitstamp.GetEuroPairs())
		_, pairUSDListData             = getPairs(bitstamp.GetUSDPairs())
		_, pairGBPListData             = getPairs(bitstamp.GetGBPPairs())
		_, pairBTCListData             = getPairs(bitstamp.GetBTCPairs())
		activePair                     = activepair.NewActivePair(pairFullList[0])
		ctx, cancel                    = context.WithCancel(context.Background())
	)
	defer cancel()

	// Initialize ui
	if err := ui.Init(); err != nil {
		fmt.Printf("failed to initialize ui: %v", err)
		return
	}
	defer ui.Close()

	// Initialize bitstamp websocket client and start consuming events
	ws, err := bitstamp.NewWebsocketAPI()
	terminateOnError("websocket client failed to connect", err)
	defer ws.Close()

	events, err := ws.Consume(ctx,
		bitstamp.GetDetailOrderBookChannel(activePair.Get()),
		bitstamp.GetLiveTradeChannel(activePair.Get()))
	terminateOnError("websocket client failed to consume socket", err)

	for i := range pairFullList {
		pairMap[pairFullList[i].String()] = pairFullList[i]
	}

	// Generic styles
	titleStyle := ui.NewStyle(ui.ColorGreen, ui.ColorClear, ui.ModifierBold)
	textStyle := ui.NewStyle(ui.ColorClear, ui.ColorClear, ui.ModifierClear)
	borderStyle := ui.NewStyle(ui.ColorGreen, ui.ColorClear, ui.ModifierBold)
	selectedRowStyle := ui.NewStyle(ui.ColorClear, ui.ColorGreen, ui.ModifierBold)
	tableHeaderStyle := ui.NewStyle(ui.ColorClear, ui.ColorClear, ui.ModifierBold)

	redText := func(s string) string {
		return fmt.Sprintf("[%s](fg:red,bg:clear)", s)
	}
	greenText := func(s string) string {
		return fmt.Sprintf("[%s](fg:green,bg:clear)", s)
	}

	cList := widgets.NewList()
	cList.Title = "| Currencies |"
	cList.Rows = currencyListData
	cList.TitleStyle = titleStyle
	cList.BorderStyle = borderStyle
	cList.SelectedRowStyle = selectedRowStyle
	cList.TextStyle = textStyle

	pList := widgets.NewList()
	pList.Title = fmt.Sprintf("| Pairs %s%s |", string(rune(8593)), string(rune(8595)))
	pList.Rows = pairFullListData
	pList.TitleStyle = titleStyle
	pList.BorderStyle = borderStyle
	pList.SelectedRowStyle = selectedRowStyle
	pList.TextStyle = textStyle

	chart := widgets.NewPlot()
	chart.Title = "| Chart (1h) |"
	chart.Data = make([][]float64, 1)
	chart.Data[0] = []float64{1, 1, 1, 1}
	chart.AxesColor = ui.ColorClear
	chart.LineColors[0] = ui.ColorGreen
	chart.BorderStyle = borderStyle
	chart.TitleStyle = titleStyle

	// update chart data for active pair
	updateChartData := func() {
		result, err := bitClient.GetOHLCData(ctx, activePair.Get(), bitstamp.GetOHLCDataRequest{
			Step:  3600,
			Limit: 48,
		})
		terminateOnError("failed to retrieve pair OHCL data", err)

		ps := make([]float64, 0, len(result.Data.Ohlc))
		for i := range result.Data.Ohlc {
			close, _ := strconv.ParseFloat(result.Data.Ohlc[i].Close, 64)
			ps = append(ps, close)
		}

		chart.Lock()
		chart.Data[0] = ps
		chart.Unlock()
	}

	// keep chart data updated
	go func() {
		ticker := time.NewTicker(time.Hour * 1)
		defer ticker.Stop()
		for ; true; <-ticker.C {
			updateChartData()
		}
	}()

	liveTrades := widgets.NewTable()
	liveTrades.Rows = [][]string{{"Amount", "Time", "Price"}}
	liveTrades.Title = "| Live Trades |"
	liveTrades.TextAlignment = ui.AlignCenter
	liveTrades.RowSeparator = false
	liveTrades.TitleStyle = titleStyle
	liveTrades.TextStyle = textStyle
	liveTrades.BorderStyle = borderStyle
	liveTrades.RowStyles[0] = tableHeaderStyle

	// Set live trades data
	setLiveTradesRows := func(data [][]string) {
		liveTrades.Lock()
		liveTrades.Rows = data
		liveTrades.Unlock()
	}

	// update live trades data
	updateLiveTradesRows := func() {
		data, err := bitClient.GetTransactions(ctx, activePair.Get(), bitstamp.GetTransactionsRequest{
			Time: "day",
		})
		terminateOnError("websocket client failed to connect", err)

		rows := [][]string{{"Amount", "Time", "Price"}}
		for i := range data {
			var t time.Time
			ts, err := strconv.ParseInt(data[0].Date, 10, 64)
			if err == nil {
				t = time.Unix(ts, 0)
			}

			price := greenText(data[i].Price)
			if data[i].Type == "1" {
				price = redText(data[i].Price)
			}

			rows = append(rows, []string{data[i].Amount, t.Format("15:04:05"), price})

		}

		setLiveTradesRows(rows)
	}
	go updateLiveTradesRows()

	// clear live trades data
	clearLiveTrades := func() {
		liveTrades.Lock()
		liveTrades.Rows = [][]string{{"Amount", "Time", "Price"}}
		liveTrades.Unlock()
	}

	orderBook := widgets.NewTable()
	orderBook.Rows = [][]string{{"Value", "Amount", "Bid", "Ask", "Amount", "Value"}}
	orderBook.Title = "| Order Book |"
	orderBook.TextAlignment = ui.AlignCenter
	orderBook.RowSeparator = false
	orderBook.TitleStyle = titleStyle
	orderBook.TextStyle = textStyle
	orderBook.BorderStyle = borderStyle
	orderBook.RowStyles[0] = tableHeaderStyle

	// set order book data
	setOrderBookRows := func(data [][]string) {
		orderBook.Lock()
		orderBook.Rows = data
		orderBook.Unlock()
	}

	// help menu
	help := widgets.NewTable()
	help.Title = "| Help |"
	help.Rows = [][]string{
		{"Command", "Key"},
		{"Select currency", "1, 2, 3, 4, 5"},
		{"Select previous pair", "up, s, mouse wheel up"},
		{"Select next pair", "down, w, mouse wheel down"},
		{"Show/Hide this menu", "h"},
		{"Quit", "q"},
	}
	help.TextAlignment = 1
	help.TextStyle = textStyle
	help.TitleStyle = titleStyle
	help.BorderStyle = borderStyle
	help.RowStyles[0] = tableHeaderStyle

	// initialize ui grid
	grid := ui.NewGrid()
	termWidth, termHeight := ui.TerminalDimensions()
	grid.SetRect(0, 0, termWidth, termHeight)
	grid.Set(
		ui.NewCol(0.3,
			ui.NewRow(0.5,
				ui.NewCol(0.5, cList),
				ui.NewCol(0.5, pList),
			),
			ui.NewRow(0.5, liveTrades),
		),
		ui.NewCol(0.7,
			ui.NewRow(0.5, chart),
			ui.NewRow(0.5, orderBook),
		),
	)
	ui.Render(grid)

	// Provides data to live trade and order book widgets
	go func() {
		for event := range events {
			terminateOnError(string(event.RawMessage), event.Error)

			switch v := event.Message.(type) {
			case bitstamp.LiveDetailOrderBookChannel:
				if strings.HasSuffix(v.Channel, activePair.Get().String()) {
					bookData := make([][]string, 0, len(v.Data.Bids))

					for i := range v.Data.Bids {
						price, _ := strconv.ParseFloat(v.Data.Bids[i][0], 64)
						amount, _ := strconv.ParseFloat(v.Data.Bids[i][1], 64)
						value := price * amount

						bookData = append(bookData, []string{fmt.Sprintf("%.2f", value), v.Data.Bids[i][1], greenText(v.Data.Bids[i][0]), "0", "0", "0"})
					}

					for i := range v.Data.Asks {
						price, _ := strconv.ParseFloat(v.Data.Asks[i][0], 64)
						amount, _ := strconv.ParseFloat(v.Data.Asks[i][1], 64)
						value := price * amount

						if len(v.Data.Bids) > i {
							bookData[i][3] = redText(v.Data.Asks[i][0])
							bookData[i][4] = v.Data.Asks[i][1]
							bookData[i][5] = fmt.Sprintf("%.2f", value)
						}
					}

					setOrderBookRows(append([][]string{{"Value", "Amount", "Bid", "Ask", "Amount", "Value"}}, bookData...))
				}

			case bitstamp.LiveTickerChannel:
				if strings.HasSuffix(v.Channel, activePair.Get().String()) {
					var t time.Time
					i, err := strconv.ParseInt(v.Data.Timestamp, 10, 64)
					if err == nil {
						t = time.Unix(i, 0)
					}

					if len(liveTrades.Rows) > 100 {
						liveTrades.Rows = liveTrades.Rows[:35]
					}

					price := greenText(v.Data.PriceStr)
					if v.Data.Type == 1 {
						price = redText(v.Data.PriceStr)
					}

					toInsert := []string{v.Data.AmountStr, t.Format("15:04:05"), price}
					setLiveTradesRows(append(liveTrades.Rows[:1], append([][]string{toInsert}, liveTrades.Rows[1:]...)...))
				}
			}
		}
	}()

	// Poll ui events
	uiEvents := ui.PollEvents()
	ticker := time.NewTicker(time.Millisecond * 50).C
	for {
		select {
		case e := <-uiEvents:
			switch e.ID {
			case "q", "Q", "<C-c>":
				return
			case "<Up>", "w", "W", "<MouseWheelUp>":
				pList.ScrollUp()
			case "<PageUp>":
				pList.ScrollPageUp()
			case "<Down>", "s", "S", "<MouseWheelDown>":
				pList.ScrollDown()
			case "<PageDown>":
				pList.ScrollPageDown()
			case "1":
				pList.Rows = pairFullListData
				cList.SelectedRow = 0
				pList.SelectedRow = 0
			case "2":
				pList.Rows = pairBTCListData
				cList.SelectedRow = 1
				pList.SelectedRow = 0
			case "3":
				pList.Rows = pairEuroListData
				cList.SelectedRow = 2
				pList.SelectedRow = 0
			case "4":
				pList.Rows = pairGBPListData
				cList.SelectedRow = 3
				pList.SelectedRow = 0
			case "5":
				pList.Rows = pairUSDListData
				cList.SelectedRow = 4
				pList.SelectedRow = 0
			case "<Resize>":
				payload := e.Payload.(ui.Resize)
				grid.SetRect(0, 0, payload.Width, payload.Height)
				ui.Clear()
				ui.Render(grid)
			// show hide help
			case "h", "H":
				r := help.GetRect()
				if r.Dx() > 0 {
					help.SetRect(0, 0, 0, 0)
					continue
				}

				w, h := ui.TerminalDimensions()
				help.SetRect(0, 0, w, h)
				ui.Render(help)
			}
		case <-ticker:
			if r := help.GetRect(); r.Size().X > 0 {
				continue
			}

			ui.Render(grid)
		}

		pairTxt := pList.Rows[pList.SelectedRow]
		selectedPair, ok := pairMap[strings.ToLower(pairTxt)]
		if !ok {
			terminateOnError(fmt.Sprintf("invalid pair %s\n", pairTxt), nil)
		}

		// Do the following only on pair change
		if activePair.Get() != selectedPair {
			activePair.Set(selectedPair)
			clearLiveTrades()
			go updateLiveTradesRows()
			go updateChartData()

			// Unsubscribe from previous subscribed channels and subscribe to new ones
			if err := ws.UnSubscribeFromAllChannels(ctx); err != nil {
				terminateOnError("", err)
			}
			if err := ws.SubscribeToChannels(ctx,
				bitstamp.GetDetailOrderBookChannel(selectedPair),
				bitstamp.GetLiveTradeChannel(selectedPair),
			); err != nil {
				terminateOnError("", err)
			}
		}
	}
}

// Accepts a slice of pairs and returns a sorted slice of pairs and a
// sorted slice of rows that can be used as list elements
func getPairs(pairs []bitstamp.Pair) ([]bitstamp.Pair, []string) {
	var elements []string

	for i := range pairs {
		elements = append(elements, strings.ToUpper(pairs[i].String()))
	}
	sort.Slice(pairs, func(i, j int) bool {
		return pairs[i] < pairs[j]
	})
	sort.Slice(elements, func(i, j int) bool {
		return elements[i] < elements[j]
	})

	return pairs, elements
}

// Terminates ui and shows error and line
func terminateOnError(message string, err error) {
	if err == nil {
		return
	}

	ui.Clear()
	ui.Close()

	_, _, line, _ := runtime.Caller(1)
	fmt.Printf("%s %s | line: %d\n", message, err, line)
	os.Exit(1)
}
