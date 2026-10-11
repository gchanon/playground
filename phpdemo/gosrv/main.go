// gosrv — server ฝั่ง Go ที่มี endpoint "หน้าตาเหมือนกันเป๊ะ" กับ srv.php
//
// มีไว้เพื่อให้เทียบกันได้แบบยุติธรรม: โหลดชุดเดียวกัน เครื่องเดียวกัน DB ตัวเดียวกัน
// ต่างกันแค่ runtime
//
//	go run ./phpdemo/gosrv            # ฟังที่ :8081
//
// endpoint ทั้งหมด (srv.php มีครบชุดเดียวกัน)
//
//	/ping        ตอบทันที — วัด overhead ของ runtime ล้วนๆ
//	/state       เพิ่มตัวนับที่อยู่ "ในโปรเซส" แล้วคืนค่า — หัวใจของ demo lifecycle
//	/db          query ผ่าน pool — คืน pid ของ backend ฝั่ง PG
//	/db1         query โดยต่อใหม่ทุก request (ท่าเดียวกับ PHP) — เอาไว้พิสูจน์ว่าที่ช้าคือ "ไม่มี pool" ไม่ใช่ "ภาษา"
//	/sleep?ms=   งานที่รอ I/O — วัดว่า runtime รับงานพร้อมกันได้กี่ชิ้น
//	/cpu?n=      งานที่กิน CPU — วัดความเร็ว compiled เทียบ interpreted
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// hits = ตัวแปรระดับโปรเซส อยู่ยาวตลอดอายุ server
// นี่คือสิ่งที่ PHP "ไม่มี" และเป็นความต่างข้อแรกที่ต้องเข้าใจ
var hits atomic.Int64

const dsn = "postgres://postgres:pgdemo@localhost:55432/pgdemo"

func main() {
	addr := ":8081"
	if v := os.Getenv("GOSRV_ADDR"); v != "" {
		addr = v
	}

	ctx := context.Background()

	// pool เปิดครั้งเดียวตอน boot แล้วใช้ซ้ำทุก request
	// ขนาด 10 ตั้งให้เท่ากับจำนวน worker ของ PHP เพื่อให้เทียบกันได้
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		log.Fatal(err)
	}
	cfg.MaxConns = 10
	cfg.MinConns = 10
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("ต่อ DB ไม่ได้: %v\n  ลืมสั่ง `docker compose up -d` ใน pgdemo/ รึเปล่า", err)
	}

	mux := http.NewServeMux()

	mux.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "pong")
	})

	// ตัวนับอยู่ในโปรเซสที่ไม่เคยตาย → เลขวิ่งขึ้นเรื่อยๆ 1, 2, 3, ...
	mux.HandleFunc("/state", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, hits.Add(1))
	})

	// ใช้ connection จาก pool — ไม่มีการ handshake ใหม่
	mux.HandleFunc("/db", func(w http.ResponseWriter, r *http.Request) {
		var pid int
		if err := pool.QueryRow(r.Context(), `SELECT pg_backend_pid()`).Scan(&pid); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		fmt.Fprint(w, pid)
	})

	// ต่อใหม่ทุก request ทิ้งทุก request — เลียนแบบ PHP แบบไม่มี persistent connection
	mux.HandleFunc("/db1", func(w http.ResponseWriter, r *http.Request) {
		conn, err := pgx.Connect(r.Context(), dsn)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		defer conn.Close(r.Context())
		var pid int
		if err := conn.QueryRow(r.Context(), `SELECT pg_backend_pid()`).Scan(&pid); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		fmt.Fprint(w, pid)
	})

	mux.HandleFunc("/sleep", func(w http.ResponseWriter, r *http.Request) {
		ms := intParam(r, "ms", 100)
		time.Sleep(time.Duration(ms) * time.Millisecond)
		fmt.Fprint(w, "slept")
	})

	mux.HandleFunc("/cpu", func(w http.ResponseWriter, r *http.Request) {
		n := intParam(r, "n", 200000)
		fmt.Fprint(w, sumPrimes(n))
	})

	log.Printf("gosrv ฟังที่ %s — pool %d เส้น", addr, cfg.MaxConns)
	log.Fatal(http.ListenAndServe(addr, mux))
}

// sumPrimes ต้องเขียนให้ "เหมือนกันเป๊ะ" กับฝั่ง PHP ไม่งั้นเทียบความเร็วไม่ได้
func sumPrimes(n int) int {
	sum := 0
	for i := 2; i < n; i++ {
		isPrime := true
		for j := 2; j*j <= i; j++ {
			if i%j == 0 {
				isPrime = false
				break
			}
		}
		if isPrime {
			sum += i
		}
	}
	return sum
}

func intParam(r *http.Request, k string, def int) int {
	if v := r.URL.Query().Get(k); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}
