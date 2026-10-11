#!/usr/bin/env bash
# run.sh — รัน demo ฝั่ง HTTP ทั้งหมด แล้วพิมพ์ตารางผลวัด
#
#   bash phpdemo/run.sh            # รันทุก demo
#   bash phpdemo/run.sh pool       # รันทีละตัว: lifecycle | ping | cpu | pool | worker | opcache
#
# ต้องมี PG ของ pgdemo รันอยู่ก่อน:  cd pgdemo && docker compose up -d

set -uo pipefail
cd "$(dirname "$0")/.."
export PATH=/opt/homebrew/bin:$PATH

WORKERS=${WORKERS:-10}      # จำนวน PHP worker — ตั้งให้เท่าขนาด pool ของ gosrv จะได้เทียบกันได้
PHP_PORT=8080
GO_PORT=8081
ONLY=${1:-all}

php_pid=""; go_pid=""
# kill_port ฆ่าทุกโปรเซสที่ถือ port นั้นอยู่
#
# ทำไมต้องฆ่าด้วย port ไม่ใช่ pkill -f ตามชื่อคำสั่ง:
# ตอนเขียน demo นี้ใช้ pkill -f "php -S 127.0.0.1:8080" แล้วมันไม่ match
# server ที่สั่งด้วย `php -d opcache.enable=0 -S ...` เพราะมี flag แทรกกลาง
# ผลคือ server เก่ารอด ถือ port ไว้ แล้ว benchmark ไปวัด server ตัวเก่าเงียบๆ
# อาการที่เห็น: ตัวเลข opcache off/on ออกมาเท่ากันแบบน่าสงสัย
kill_port() {
    local pids
    pids=$(lsof -ti "tcp:$1" 2>/dev/null)
    [[ -n "$pids" ]] && kill $pids 2>/dev/null
    return 0
}

cleanup() {
    kill_port "$PHP_PORT"
    kill_port "$GO_PORT"
    return 0
}
trap cleanup EXIT
# ไม่ใช้ `wait` ใน cleanup — php -S แตก worker ลูกออกไป พ่อตายแล้วลูกยังอยู่
# bash จะค้างรอ job ที่ไม่มีวันจบ (เจอมาแล้วตอนเขียน demo นี้เหมือนกัน)

want() { [[ "$ONLY" == "all" || "$ONLY" == "$1" ]]; }
hdr()  { printf '\n\n══════ %s ══════\n' "$1"; }
sec()  { printf '\n── %s\n\n' "$1"; }
note() { printf '   » %s\n' "$1"; }

wait_up() {   # wait_up <url> — รอจน server ตอบ ไม่ใช่ sleep มั่วๆ
    for _ in $(seq 1 50); do
        curl -fsS "$1" >/dev/null 2>&1 && return 0
        perl -e 'select(undef,undef,undef,0.2)'
    done
    echo "!! server ไม่ขึ้น: $1" >&2; return 1
}

port_free() {  # รอจน port ว่างจริง
    for _ in $(seq 1 50); do
        lsof -ti "tcp:$1" -sTCP:LISTEN >/dev/null 2>&1 || return 0
        perl -e 'select(undef,undef,undef,0.2)'
    done
    echo "!! port $1 ไม่ยอมว่าง" >&2; return 1
}

start_php() {  # start_php [extra php -d flags...]
    # ต้องมั่นใจว่า server เก่าตายและ port ว่างจริงก่อน
    # ไม่งั้น server ใหม่ bind ไม่ได้ แล้วเราจะวัด server เก่าโดยไม่รู้ตัว
    # (ตัวเลข opcache=off/on จะออกมาเท่ากันเป๊ะแบบน่าสงสัย — นั่นคืออาการ)
    kill_port "$PHP_PORT"
    port_free "$PHP_PORT" || return 1

    PHP_CLI_SERVER_WORKERS=$WORKERS php "$@" -S 127.0.0.1:$PHP_PORT phpdemo/srv.php \
        >/tmp/phpdemo-php.log 2>&1 &
    php_pid=$!
    wait_up "http://127.0.0.1:$PHP_PORT/ping"
}

bench() { go run ./phpdemo/bench "$@"; }

# ---------- เตรียม ----------
php phpdemo/setup.php || { echo "!! ต่อ PG ไม่ได้ — ลืม docker compose up -d ใน pgdemo/ รึเปล่า"; exit 1; }

go build -o /tmp/phpdemo-gosrv ./phpdemo/gosrv || exit 1
kill_port "$GO_PORT"
port_free "$GO_PORT" || exit 1
/tmp/phpdemo-gosrv >/tmp/phpdemo-go.log 2>&1 &
go_pid=$!
wait_up "http://127.0.0.1:$GO_PORT/ping" || exit 1

start_php || exit 1

printf '\n=== PHP: %s\n' "$(curl -s http://127.0.0.1:$PHP_PORT/info)"
printf '=== Go : %s worker=%s pool=10\n' "$(go version | awk '{print $3}')" "$WORKERS"


# ═══════════════════════════════════════════════════════════════
# A1 · lifecycle — PHP ลืมทุกอย่างตอนจบ request
# ═══════════════════════════════════════════════════════════════
if want lifecycle; then
hdr "A1 · ตัวแปรในโปรเซส: Go จำได้ PHP ลืมทุกครั้ง"
sec "ยิง /state 5 ครั้งติดกัน"
printf '   PHP: '; for _ in 1 2 3 4 5; do printf '[%s] ' "$(curl -s http://127.0.0.1:$PHP_PORT/state)"; done; echo
printf '   Go : '; for _ in 1 2 3 4 5; do printf '[%s] ' "$(curl -s http://127.0.0.1:$GO_PORT/state)"; done; echo
note ""
note "PHP: static=1 ทุกครั้ง — ไฟล์ srv.php ถูกรันใหม่ตั้งแต่บรรทัดแรกทุก request"
note "     pid เปลี่ยนไปมา เพราะ worker คนละตัวรับงาน"
note "     shared=... วิ่งขึ้นได้ เพราะเก็บไว้ใน PG ซึ่งอยู่ 'นอกโปรเซส'"
note "Go : ตัวนับวิ่งขึ้น 1,2,3,... เพราะ atomic.Int64 อยู่ในโปรเซสที่ไม่เคยตาย"
note ""
note "ผลที่ตามมาในงานจริง:"
note "  · in-memory cache ใน PHP ไม่มีความหมาย — ต้องไป Redis เสมอ"
note "  · singleton / lazy init ใน PHP = สร้างใหม่ทุก request ไม่ได้ประหยัดอะไร"
note "  · แต่ก็แปลว่า PHP ไม่มี memory leak ข้าม request และไม่มี data race ระหว่าง request"
fi


# ═══════════════════════════════════════════════════════════════
# A2 · ping — overhead ของ runtime ล้วนๆ
# ═══════════════════════════════════════════════════════════════
if want ping; then
hdr "A2 · ราคาของการ 'เริ่มใหม่ทุก request'"
sec "/ping — ไม่ทำอะไรเลย ตอบ pong อย่างเดียว"
bench -url "http://127.0.0.1:$PHP_PORT/ping" -c 10 -d 3s -label "PHP (w=$WORKERS)"
bench -url "http://127.0.0.1:$GO_PORT/ping"  -c 10 -d 3s -label "Go"
note ""
note "ตัวเลขนี้คือ 'ค่าเข้าประตู' ของ PHP: boot + parse + รันไฟล์ใหม่ + ล้างทิ้ง"
note "Go จ่ายค่านี้ครั้งเดียวตอน start server"
fi


# ═══════════════════════════════════════════════════════════════
# A3 · cpu — ภาษา interpret เทียบ compile
# ═══════════════════════════════════════════════════════════════
if want cpu; then
hdr "A3 · งานกิน CPU — อัลกอริทึมเดียวกัน บรรทัดต่อบรรทัด"
sec "/cpu?n=200000 — หาผลรวมจำนวนเฉพาะด้วยลูปซ้อนลูป"
bench -url "http://127.0.0.1:$PHP_PORT/cpu?n=200000" -c 4 -n 20 -warmup 2 -label "PHP"
bench -url "http://127.0.0.1:$GO_PORT/cpu?n=200000"  -c 4 -n 20 -warmup 2 -label "Go"
note ""
note "โค้ดเหมือนกันเป๊ะ (sumPrimes ใน lib.php กับ gosrv/main.go)"
note "ช่องว่างตรงนี้คือราคาของ interpreter เทียบกับ machine code"
note "แต่ในงาน marketplace จริง เวลาส่วนใหญ่หมดไปกับรอ DB/API ไม่ใช่ลูปแบบนี้"
fi


# ═══════════════════════════════════════════════════════════════
# A4 · pool — ความต่างที่แพงที่สุดในชีวิตจริง
# ═══════════════════════════════════════════════════════════════
if want pool; then
hdr "A4 · connection pool — สิ่งที่ PHP ไม่มีให้"
sec "/db คืน pg_backend_pid() · นับ 'คำตอบไม่ซ้ำ' = ใช้ connection ไปกี่เส้น"
bench -url "http://127.0.0.1:$PHP_PORT/db"  -c 10 -n 300 -warmup 5 -uniq -label "PHP ต่อใหม่ทุก req"
bench -url "http://127.0.0.1:$PHP_PORT/dbp" -c 10 -n 300 -warmup 5 -uniq -label "PHP persistent"
bench -url "http://127.0.0.1:$GO_PORT/db"   -c 10 -n 300 -warmup 5 -uniq -label "Go pool(10)"
bench -url "http://127.0.0.1:$GO_PORT/db1"  -c 10 -n 300 -warmup 5 -uniq -label "Go ต่อใหม่ทุก req"
note ""
note "อ่านผล:"
note "  · PHP /db  กับ Go /db1 ช้าพอๆ กัน → ที่ช้าคือ 'ไม่มี pool' ไม่ใช่ 'ภาษา PHP ช้า'"
note "  · PHP /dbp ใช้ connection เท่าจำนวน worker — worker ตัวเดิมถือเส้นเดิมไว้"
note "  · Go /db   ใช้เท่าขนาด pool ที่เปิดไว้ตอน boot"
note ""
note "ของจริงบน prod: php-fpm 200 children x 10 server = 2000 connection ที่ PG"
note "PG รับไม่ไหว (1 connection = 1 process) → ต้องมี pgbouncer คั่นกลาง"
note "Go ไม่ต้อง เพราะคุม pool ได้จากในแอปเอง — นี่คือเหตุผลเชิงสถาปัตยกรรม ไม่ใช่รสนิยม"
fi


# ═══════════════════════════════════════════════════════════════
# A5 · worker — ไม่มี goroutine แล้วรับงานพร้อมกันได้เท่าไหร่
# ═══════════════════════════════════════════════════════════════
if want worker; then
hdr "A5 · งานที่รอ I/O — PHP รับพร้อมกันได้เท่าจำนวน worker เป๊ะๆ"
sec "/sleep?ms=100 — จำลองเรียก API ข้างนอกที่ใช้เวลา 100ms"
for c in 5 10 20 40; do
    bench -url "http://127.0.0.1:$PHP_PORT/sleep?ms=100" -c $c -d 3s -warmup 2 -label "PHP (w=$WORKERS)"
done
echo
for c in 5 10 20 40; do
    bench -url "http://127.0.0.1:$GO_PORT/sleep?ms=100" -c $c -d 3s -warmup 2 -label "Go"
done
note ""
note "PHP: throughput ตันที่ ~$WORKERS x 10 req/s = เท่าจำนวน worker หารด้วยเวลางาน"
note "     ยิงเกิน $WORKERS ตัวพร้อมกัน ที่เกินมาไม่ได้ทำงาน แค่ต่อคิวอยู่ → p99 พุ่ง"
note "Go : throughput โตตามที่ยิง เพราะ goroutine ที่ sleep ไม่กิน OS thread"
note ""
note "นี่คือเหตุผลที่ PHP ต้องแตก worker เยอะ และทำไมงานช้าต้องโยนเข้า queue"
note "1 request ที่รอ API ข้างนอก 3 วินาที = ยึด worker ไป 1 ตัวเต็มๆ 3 วินาที"
fi


# ═══════════════════════════════════════════════════════════════
# A6 · opcache — PHP compile ใหม่ทุก request จริงมั้ย
# ═══════════════════════════════════════════════════════════════
if want opcache; then
hdr "A6 · opcache — แปลงเป็น bytecode ใหม่ทุก request จริงมั้ย"
php phpdemo/gen_vendor.php 300 >/dev/null || exit 1

sec "/ping — ไฟล์เดียว 150 บรรทัด"
start_php -d opcache.enable=0 || exit 1
bench -url "http://127.0.0.1:$PHP_PORT/ping" -c 10 -d 3s -label "opcache=off"
start_php -d opcache.enable=1 || exit 1
bench -url "http://127.0.0.1:$PHP_PORT/ping" -c 10 -d 3s -label "opcache=on"
note ""
note "ต่างกันไม่มาก — ไฟล์เดียวเล็กๆ parse ถูกอยู่แล้ว opcache เลยไม่มีอะไรให้ประหยัดเท่าไหร่"
note "ถ้าหยุดวัดแค่ตรงนี้จะสรุปผิดว่า 'opcache แทบไม่ช่วย' — ดูบล็อกถัดไป"

sec "/heavy — require 300 ไฟล์ต่อ request (จำลอง vendor/ ของ framework)"
start_php -d opcache.enable=0 || exit 1
bench -url "http://127.0.0.1:$PHP_PORT/heavy" -c 10 -d 3s -warmup 5 -label "opcache=off"
start_php -d opcache.enable=1 || exit 1
bench -url "http://127.0.0.1:$PHP_PORT/heavy" -c 10 -d 3s -warmup 5 -label "opcache=on"
note ""
note "ตรงนี้ถึงเห็นผล — opcache เก็บ bytecode ที่แปลแล้วไว้ใน shared memory"
note "ที่ worker ทุกตัวเห็นร่วมกัน จึงไม่ต้อง parse ไฟล์ใหม่ทุก request"
note "Laravel ของจริง autoload หลายร้อยถึงพันไฟล์ต่อ request — ใกล้เคียงกรณีนี้มากกว่า /ping"
note ""
note "เทียบ Go: งาน parse+compile จ่ายครั้งเดียวตอน go build"
note "opcache คือการพยายามเลียนแบบเรื่องนี้ แต่ยังต้องเช็ค mtime ของไฟล์อยู่ดี"
note ""
note "จุดที่ต้องแยกให้ออก (ข้อนี้ถูกถาม interview บ่อย):"
note "  opcache แคช 'โค้ดที่แปลแล้ว' — ข้าม request ได้"
note "  ส่วน 'สถานะของโปรแกรม' ไม่เคยข้าม request — ต้องไป Redis/DB เสมอ"
note "บน prod ต้องเปิด opcache เสมอ ปิดไว้คือจ่ายค่า parse ฟรีๆ ทุก request"
fi

echo
