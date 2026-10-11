package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// ต่อผ่าน TCP ไปที่ container — ต่างจาก YottaDB ที่แอปเรียก engine ในโปรเซสตัวเอง
// ทุก connection = 1 backend process ฝั่ง server
const defaultDSN = "postgres://postgres:pgdemo@localhost:55432/pgdemo"

func dsn() string {
	if v := os.Getenv("PGDEMO_DSN"); v != "" {
		return v
	}
	return defaultDSN
}

func connect(ctx context.Context) *pgx.Conn {
	c, err := pgx.Connect(ctx, dsn())
	if err != nil {
		fmt.Fprintf(os.Stderr, "\n!! ต่อ DB ไม่ได้: %v\n   ลืมสั่ง `docker compose up -d` ใน pgdemo/ รึเปล่า\n", err)
		os.Exit(1)
	}
	return c
}

func must(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "\n!! %v\n", err)
		os.Exit(1)
	}
}

func exec(ctx context.Context, c *pgx.Conn, sql string, args ...any) {
	_, err := c.Exec(ctx, sql, args...)
	if err != nil {
		must(fmt.Errorf("%w\n   sql: %s", err, strings.TrimSpace(sql)))
	}
}

func scan1[T any](ctx context.Context, c *pgx.Conn, sql string, args ...any) T {
	var v T
	if err := c.QueryRow(ctx, sql, args...).Scan(&v); err != nil {
		must(fmt.Errorf("%w\n   sql: %s", err, strings.TrimSpace(sql)))
	}
	return v
}

// ---------- ตัววัด ----------

// relSize = ขนาดไฟล์ heap จริงของตาราง (ไม่รวม index) หน่วย byte
// PG เก็บเป็น "หน้า" ละ 8KB เสมอ ตัวเลขนี้จึงหารด้วย 8192 ลงตัวตลอด
func relSize(ctx context.Context, c *pgx.Conn, t string) int64 {
	return scan1[int64](ctx, c, `SELECT pg_relation_size($1)`, t)
}

func idxSize(ctx context.Context, c *pgx.Conn, t string) int64 {
	return scan1[int64](ctx, c, `SELECT pg_indexes_size($1)`, t)
}

// pgstattuple เดินอ่านทั้งตารางแล้วนับจริง — ช้ากว่า pg_stat_user_tables แต่ไม่ใช่ค่าประมาณ
func pgstat(ctx context.Context, c *pgx.Conn, t string) (live, dead int64) {
	must(c.QueryRow(ctx,
		`SELECT tuple_count, dead_tuple_count FROM pgstattuple($1)`, t).Scan(&live, &dead))
	return
}

// walLSN = ตำแหน่งหัวปากกาใน WAL ตอนนี้
// จับหัว-ท้ายแล้วลบกัน = งานชิ้นนี้ทำให้ PG ต้องเขียน WAL กี่ byte
func walLSN(ctx context.Context, c *pgx.Conn) string {
	return scan1[string](ctx, c, `SELECT pg_current_wal_lsn()::text`)
}

func walDiff(ctx context.Context, c *pgx.Conn, from, to string) int64 {
	return scan1[int64](ctx, c,
		`SELECT pg_wal_lsn_diff($1::pg_lsn, $2::pg_lsn)::bigint`, to, from)
}

// ---------- จัดรูป ----------

func fmtBytes(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.0f KB", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%d B", n)
	}
}

func pages(n int64) int64 { return n / 8192 }

func rate(n int, d time.Duration) float64 { return float64(n) / d.Seconds() }

func header(s string) { fmt.Printf("\n\n══════ %s ══════\n", s) }

func section(s string) { fmt.Printf("\n── %s\n", s) }

func note(s string) {
	if s == "" {
		fmt.Println()
		return
	}
	fmt.Printf("   » %s\n", s)
}

// fmtDur พิมพ์ระยะเวลาที่อาจเล็กจนจมสัญญาณรบกวน (ติดลบได้) ให้อ่านรู้เรื่อง
func fmtDur(d time.Duration) string {
	if d < 5*time.Microsecond {
		return "~0 (จมใน noise)"
	}
	return d.Round(time.Microsecond).String()
}
