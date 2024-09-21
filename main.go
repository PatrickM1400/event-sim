package main

import (
	"bufio"
	"container/list"
	"fmt"
	"log"
	"math"
	"os"
	"strconv"

	"github.com/go-echarts/go-echarts/v2/charts"
	"github.com/go-echarts/go-echarts/v2/opts"
	"github.com/iti/cmdline"
	"github.com/iti/evt/evtm"
	"github.com/iti/evt/vrtime"
	"github.com/iti/rngstream"
)

// SecondPerTick gives a float64 representation of the tick size in seconds.  Default 0.1 ns
var SecondPerTick float64 = 1e-6 // We want million ticks per second

// TicksPerSecond specifies the frequency of the ticker.  Default is 1e9.
var TicksPerSecond int64 = int64(1.0 / SecondPerTick)

type PktArrivalContext struct {
	ServiceTime         int64
	Tagged              bool
	EventDataFile       *os.File
	EventDataFileTagged *os.File
}

type PktServiceFinishContext struct {
	ServiceTime int64
}

var EventList *list.List = list.New()

func ParseCmd(cmd_parser *cmdline.CmdParser) (float64, int64, int64) {
	cmd_parser.AddFlag(2, "p", true)
	cmd_parser.AddFlag(1, "m", true)
	cmd_parser.AddFlag(1, "d", true)

	if !cmd_parser.ParseFromCmdLine() {
		fmt.Println("Command Line Parse Failed")
		os.Exit(-1)
	}

	if !cmd_parser.IsLoaded("p") {
		fmt.Println("Required Command Line Argument Missing")
		os.Exit(-1)
	}
	if !cmd_parser.IsLoaded("m") {
		fmt.Println("Required Command Line Argument Missing")
		os.Exit(-1)
	}
	if !cmd_parser.IsLoaded("d") {
		fmt.Println("Required Command Line Argument Missing")
		os.Exit(-1)
	}

	var p_value float64 = cmd_parser.GetVar("p").(float64)
	var M_value int64 = cmd_parser.GetVar("m").(int64)
	var D_value int64 = cmd_parser.GetVar("d").(int64)

	fmt.Println("Starting event sim with following parameters")
	// fmt.Println("Probability of tagged event: ", reflect.TypeOf(p_value))
	// fmt.Println("Arrival Rate: ", reflect.TypeOf(M_value))
	// fmt.Println("Service Time: ", reflect.TypeOf(D_value))

	return p_value, M_value, D_value
}

/*
All packets when initally scheduled are set with this handler
Checks if packet can be immediately serviced or should be added to queue
*/
func PktArrivalHandler(evtmgr *evtm.EventManager, context any, data any) any {

	var file *os.File = context.(PktArrivalContext).EventDataFile
	var file_tagged *os.File = context.(PktArrivalContext).EventDataFileTagged
	var str string = strconv.Itoa(EventList.Len())
	file.WriteString(str + "\n")
	if context.(PktArrivalContext).Tagged {
		file_tagged.WriteString(str + "\n")
	}

	if EventList.Front() == nil { /* Case where packet can be immediately serviced */
		tmp_context := context.(PktArrivalContext)
		tmp_time := vrtime.Time{TickCnt: tmp_context.ServiceTime, Priority: 0}
		new_context := PktServiceFinishContext{ServiceTime: tmp_context.ServiceTime}
		EventList.PushBack(new_context)
		evtmgr.Schedule(new_context, nil, PktServiceFinishHandler, tmp_time)
	} else { /* Case where packet gets added to queue */
		tmp_context := context.(PktArrivalContext)
		new_context := PktServiceFinishContext{ServiceTime: tmp_context.ServiceTime}
		EventList.PushBack(new_context)
		// fmt.Println("Event added to queue")
	}

	// fmt.Println("Arrival Packet")
	return nil
}

func PktServiceFinishHandler(evtmgr *evtm.EventManager, context any, data any) any {

	event_element := EventList.Front()
	if event_element != nil {
		EventList.Remove(event_element)
		next_event_element := EventList.Front()
		if next_event_element != nil { /* Case where another packet in queue to that packet is serviced */
			tmp_context := next_event_element.Value.(PktServiceFinishContext)
			new_context := PktServiceFinishContext{ServiceTime: tmp_context.ServiceTime}
			new_time := vrtime.Time{TickCnt: tmp_context.ServiceTime, Priority: 0}
			evtmgr.Schedule(new_context, nil, PktServiceFinishHandler, new_time)
		}
	}

	// fmt.Println("Packet finished")
	return nil
}

func generatePMF(num_categories int, file_path string) []float64 {

	freq_count := make([]float64, num_categories)
	total_events := float64(0)

	file, err := os.Open(file_path)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	// optionally, resize scanner's capacity for lines over 64K, see next example
	for scanner.Scan() {
		// fmt.Println(scanner.Text())
		event_count, err := strconv.Atoi(scanner.Text())
		if err != nil {
			fmt.Println("Unable to read from data file")
			panic(err)
		}
		freq_count[event_count] += 1
		total_events += 1

	}

	if err := scanner.Err(); err != nil {
		log.Fatal(err)
	}

	for i := 0; i < num_categories; i++ {
		freq_count[i] = freq_count[i] / total_events
	}

	return freq_count
}

func generateCDF(num_categories int, file_path string) []float64 {

	distro := generatePMF(num_categories, file_path)

	for i := 1; i < num_categories; i++ {
		distro[i] += distro[i-1]
	}
	return distro
}

func generateBarItems(num_categories int, file_path string) []opts.BarData {
	items := make([]opts.BarData, 0)
	pmf_distribution := generatePMF(num_categories, file_path)

	for i := 0; i < num_categories; i++ {
		items = append(items, opts.BarData{Value: pmf_distribution[i]})
	}

	return items

}

func main() {

	cmd_parser := cmdline.NewCmdParser()
	p_value, M_value, D_value := ParseCmd(cmd_parser)

	g1 := rngstream.New("Poisson")
	g2 := rngstream.New("Tagged")
	num_seconds := 60                           // How many seconds simulation lasts
	number_events := int(M_value) * num_seconds //M is given as events per second
	// event_times := make([]int64, number_events)

	evtmgr := evtm.New()

	/* Prepare file for data */
	file, err := os.Create("event_data.txt")
	if err != nil {
		fmt.Println("Unable to create data file")
		panic(err)
	}
	file_tagged, err := os.Create("event_data_tagged.txt")
	if err != nil {
		fmt.Println("Unable to create data file")
		panic(err)
	}

	// var service_time time.Duration = time.Duration(D_value)
	var ticks_in_simulation int = int(TicksPerSecond) * num_seconds

	for i := 0; i < number_events; i++ {
		tmp_time := vrtime.Time{TickCnt: int64((g1.RandInt(0, ticks_in_simulation))), Priority: 0}
		var tagged bool = false
		if g2.RandU01() < p_value {
			tagged = true
		}
		tmp_context := PktArrivalContext{ServiceTime: D_value, Tagged: tagged, EventDataFile: file, EventDataFileTagged: file_tagged}
		evtmgr.Schedule(tmp_context, nil, PktArrivalHandler, tmp_time)
	}

	fmt.Println(number_events, " events scheduled")

	// fmt.Println(event_times)
	// slices.Sort(event_times)
	// fmt.Println(event_times)

	fmt.Println("Probability of tagged event: ", p_value)
	fmt.Println("Arrival Rate in events/sec: ", M_value)
	fmt.Println("Service Time in ms: ", D_value)

	evtmgr.Run(float64(ticks_in_simulation))

	file_find_max, err := os.Open("event_data.txt")
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	var max_count int = 0
	scanner := bufio.NewScanner(file_find_max)
	for scanner.Scan() {
		event_count, err := strconv.Atoi(scanner.Text())
		if err != nil {
			fmt.Println("Unable to read from data file")
			panic(err)
		}
		if max_count < event_count {
			max_count = event_count
		}

	}

	max_count += 1

	// create a new bar instance
	bar := charts.NewBar()
	// set some global options like Title/Legend/ToolTip or anything else
	bar.SetGlobalOptions(charts.WithTitleOpts(opts.Title{
		Title: "Event Frequency Distribution",
	}))

	// Put data into instance
	x_axis := make([]int, max_count)
	for i := 0; i < max_count; i++ {
		x_axis[i] = i
	}

	bar.SetXAxis(x_axis).
		AddSeries("Total", generateBarItems(max_count, "event_data.txt")).
		AddSeries("Tagged", generateBarItems(max_count, "event_data_tagged.txt"))

	// Where the magic happens
	f, _ := os.Create("bar.html")
	bar.Render(f)
	fmt.Println("\nCreated distibution chart")

	/* Run KS Test */
	total_cdf := generateCDF(max_count, "event_data.txt")
	tagged_cdf := generateCDF(max_count, "event_data_tagged.txt")
	var largest_dist float64 = 0

	for i := 0; i < max_count; i++ {
		var dist float64 = math.Abs(total_cdf[i] - tagged_cdf[i])
		if dist > largest_dist {
			largest_dist = dist
		}
	}
	fmt.Println("KS Statistic: ", largest_dist)
}
