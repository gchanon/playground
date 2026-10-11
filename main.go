package main

import (
	"fmt"
	"sync"
	"time"
)

// Struct สำหรับเก็บข้อมูล
type Data struct {
	Field1 string
	Field2 int
	Field3 float64
}

// ฟังก์ชันสำหรับ print data อย่างปลอดภัย
func printDataSafely(data Data, workerID int, mu *sync.Mutex) {
	// ใช้ mutex เพื่อป้องกัน race condition ตอน print
	mu.Lock()
	defer mu.Unlock()

	fmt.Printf("=== Worker %d ===\n", workerID)
	fmt.Printf("Field1: %s\n", data.Field1)
	fmt.Printf("Field2: %d\n", data.Field2)
	fmt.Printf("Field3: %.2f\n", data.Field3)
	fmt.Println("================")
}

func exampleChannelWithStruct() {
	// สร้าง channel สำหรับ Data
	dataCh := make(chan Data, 5)
	var wg sync.WaitGroup
	var printMutex sync.Mutex // Mutex สำหรับป้องกัน race condition ตอน print

	fmt.Println("=== Channel with Struct Example ===")

	// Producer: ส่งข้อมูลเข้า channel
	go func() {
		defer close(dataCh)

		sampleData := []Data{
			{"Alice", 25, 1000.50},
			{"Bob", 30, 2500.75},
			{"Charlie", 28, 1800.25},
			{"Diana", 32, 3200.00},
			{"Eve", 27, 1950.80},
		}

		for _, data := range sampleData {
			dataCh <- data
			fmt.Printf("Producer: Sent data for %s\n", data.Field1)
			time.Sleep(200 * time.Millisecond) // จำลองการส่งข้อมูลแบบช้าๆ
		}
		fmt.Println("Producer: All data sent")
	}()

	// Consumer: สร้าง workers หลายตัวเพื่อดึงข้อมูลจาก channel
	numWorkers := 3
	for i := 1; i <= numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			// อ่านข้อมูลจาก channel จนกว่าจะปิด
			for data := range dataCh {
				fmt.Printf("Worker %d: Received data for %s\n", workerID, data.Field1)

				// จำลองการประมวลผล
				time.Sleep(500 * time.Millisecond)

				// Print ข้อมูลอย่างปลอดภัย (ป้องกัน race condition)
				printDataSafely(data, workerID, &printMutex)
			}
			fmt.Printf("Worker %d: Finished\n", workerID)
		}(i)
	}

	// รอให้ทุก worker เสร็จ
	wg.Wait()
	fmt.Println("All workers completed!")
}

// ตัวอย่างที่ 2: การใช้ buffered channel แบบ pre-loaded
func examplePreloadedChannel() {
	fmt.Println("\n=== Pre-loaded Channel Example ===")

	// สร้าง channel และใส่ข้อมูลล่วงหน้า
	dataCh := make(chan Data, 3)
	dataCh <- Data{"Product A", 100, 99.99}
	dataCh <- Data{"Product B", 50, 149.50}
	dataCh <- Data{"Product C", 75, 299.00}
	close(dataCh) // ปิด channel เพราะไม่มีการส่งข้อมูลเพิ่ม

	var wg sync.WaitGroup
	var printMutex sync.Mutex

	// สร้าง workers เพื่อประมวลผลข้อมูลที่มีอยู่
	for i := 1; i <= 2; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			for data := range dataCh {
				fmt.Printf("Worker %d: Processing %s\n", workerID, data.Field1)

				// จำลองการประมวลผล
				time.Sleep(300 * time.Millisecond)

				// Print อย่างปลอดภัย
				printMutex.Lock()
				fmt.Printf("--- Worker %d Results ---\n", workerID)
				fmt.Printf("Name: %s\n", data.Field1)
				fmt.Printf("Quantity: %d\n", data.Field2)
				fmt.Printf("Price: $%.2f\n", data.Field3)
				fmt.Printf("Total Value: $%.2f\n", float64(data.Field2)*data.Field3)
				fmt.Println("------------------------")
				printMutex.Unlock()
			}
		}(i)
	}

	wg.Wait()
	fmt.Println("Pre-loaded channel processing completed!")
}

// ตัวอย่างที่ 3: การใช้ select statement
func exampleSelectStatement() {
	fmt.Println("\n=== Select Statement Example ===")

	ch1 := make(chan Data, 2)
	ch2 := make(chan Data, 2)

	// เตรียมข้อมูลใน channels
	ch1 <- Data{"Task 1A", 10, 1.1}
	ch1 <- Data{"Task 1B", 20, 2.2}

	ch2 <- Data{"Task 2A", 30, 3.3}
	ch2 <- Data{"Task 2B", 40, 4.4}

	close(ch1)
	close(ch2)

	var printMutex sync.Mutex

	// ใช้ select เพื่อรับข้อมูลจากหลาย channels
	for {
		select {
		case data, ok := <-ch1:
			if !ok {
				ch1 = nil // ตั้งค่าเป็น nil เพื่อไม่ให้ select เลือก channel นี้อีก
				break
			}
			printMutex.Lock()
			fmt.Printf("From Channel 1: %s, %d, %.1f\n", data.Field1, data.Field2, data.Field3)
			printMutex.Unlock()

		case data, ok := <-ch2:
			if !ok {
				ch2 = nil
				break
			}
			printMutex.Lock()
			fmt.Printf("From Channel 2: %s, %d, %.1f\n", data.Field1, data.Field2, data.Field3)
			printMutex.Unlock()
		}

		// ถ้าทั้งสอง channels ปิดแล้ว ให้หยุด loop
		if ch1 == nil && ch2 == nil {
			break
		}
	}

	fmt.Println("Select statement processing completed!")
}

func main() {
	// รันตัวอย่างทั้งหมด
	exampleChannelWithStruct()
	examplePreloadedChannel()
	exampleSelectStatement()
}
