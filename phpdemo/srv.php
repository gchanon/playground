<?php
declare(strict_types=1);

// srv.php — server ฝั่ง PHP ที่มี endpoint หน้าตาเหมือน gosrv/main.go เป๊ะ
//
//   PHP_CLI_SERVER_WORKERS=10 php -S 127.0.0.1:8080 phpdemo/srv.php
//
// สิ่งที่ต้องเข้าใจก่อนอ่านไฟล์นี้ และเป็นแก่นของทั้ง phpdemo:
//
//   Go  : โปรเซสเดียวเปิดค้าง → main() รันครั้งเดียว → handler ถูกเรียกซ้ำ
//         ตัวแปรนอก handler อยู่ยาวตลอดอายุ server
//
//   PHP : ทุก request = รันไฟล์นี้ "ตั้งแต่บรรทัดแรกใหม่ทั้งหมด"
//         จบ request = ล้างหน่วยความจำทิ้งทั้งก้อน
//         ไม่มีอะไรข้าม request ได้เลย นอกจากของที่อยู่นอกโปรเซส (DB / Redis / ไฟล์)
//
// อุปมา: Go = ร้านที่เปิดค้างทั้งวัน พนักงานจำลูกค้าเก่าได้
//        PHP = ร้านที่รื้อร้านทิ้งแล้วสร้างใหม่ทุกครั้งที่ลูกค้าเดินเข้ามา
//              พนักงานใหม่เอี่ยมทุกคน ไม่เคยเห็นใครมาก่อน
//
// ข้อดีที่คนมักลืม: เพราะลืมทุกอย่าง PHP เลย "ไม่มี memory leak ข้าม request"
// และ "ไม่มี race condition ระหว่าง request" — ของสองอย่างที่ Go ต้องระวังตลอด

require __DIR__ . '/lib.php';

$path = parse_url($_SERVER['REQUEST_URI'] ?? '/', PHP_URL_PATH) ?: '/';

header('Content-Type: text/plain; charset=utf-8');

match ($path) {
    '/ping'     => print('pong'),
    '/state'    => stateHandler(),
    '/db'       => dbHandler(false),
    '/dbp'      => dbHandler(true),
    '/sleep'    => sleepHandler(),
    '/cpu'      => cpuHandler(),
    '/heavy'    => heavyHandler(),
    '/info'     => infoHandler(),
    default     => notFound(),
};

// ─────────────────────────────────────────────────────────────
// /state — ตัวนับที่ "น่าจะ" โตขึ้นเรื่อยๆ แต่ไม่โต
//
// ลองนึกแบบ Go: var hits atomic.Int64 นอก handler → ยิง 1000 ครั้งได้ 1000
// PHP: ยิงกี่ครั้งก็ได้ 1 เพราะไฟล์ถูกรันใหม่หมดทุกครั้ง
//
// static ใน PHP = "อยู่ยาวตลอดการรันครั้งนี้" ไม่ใช่ "อยู่ยาวตลอดอายุ server"
// ซึ่งการรันครั้งนี้ = request เดียว
// ─────────────────────────────────────────────────────────────
function stateHandler(): void {
    static $hits = 0;      // เหมือน var hits int ใน Go? ไม่เหมือน — รีเซ็ตทุก request
    $hits++;

    // ของที่ข้าม request ได้จริงๆ ต้องอยู่ "นอกโปรเซส"
    // ของจริงใช้ Redis/Memcached — ที่นี่ใช้ PG ตัวเดียวกับ pgdemo จะได้ไม่ต้องลงอะไรเพิ่ม
    // ประเด็นไม่ใช่ว่าเก็บที่ไหน แต่คือ "ต้องออกไปนอกโปรเซส" เท่านั้นถึงจะข้าม request ได้
    $shared = pdo()->query(
        'UPDATE phpdemo_counter SET n = n + 1 WHERE k = \'hits\' RETURNING n'
    )->fetchColumn();

    echo "static=$hits shared=$shared pid=" . getmypid();
}

// ─────────────────────────────────────────────────────────────
// /db และ /dbp — ความต่างที่แพงที่สุดในชีวิตจริง
//
// pg_backend_pid() คือเลขประจำตัวของ backend process ฝั่ง PG
// 1 connection = 1 backend process (ตามที่เขียนไว้ใน pgdemo/util.go)
//
// ยิง 1000 request แล้วนับว่าได้ pid ไม่ซ้ำกี่ตัว:
//   PHP /db   → เกือบ 1000 ตัว   = ต่อใหม่ทิ้งใหม่ทุก request
//   PHP /dbp  → เท่าจำนวน worker = worker ตัวเดิมถือเส้นเดิมไว้
//   Go  /db   → เท่าขนาด pool    = เปิดครั้งเดียวตอน boot
// ─────────────────────────────────────────────────────────────
function dbHandler(bool $persistent): void {
    try {
        $db  = pdo($persistent);
        $pid = $db->query('SELECT pg_backend_pid()')->fetchColumn();
        echo $pid;
    } catch (PDOException $e) {
        http_response_code(500);
        echo 'ต่อ DB ไม่ได้: ' . $e->getMessage();
    }
}

// ─────────────────────────────────────────────────────────────
// /sleep — จำลองงานที่รอ I/O (เรียก API ข้างนอก / query ที่ช้า)
//
// Go  : goroutine ที่ sleep ไม่กิน OS thread — รับพร้อมกันหมื่นตัวก็ได้
// PHP : worker ที่ sleep ถูกยึดทั้งตัว — รับพร้อมกันได้ "เท่าจำนวน worker" เป๊ะๆ
//       ที่เกินมาต่อคิวอยู่หน้าประตู
// ─────────────────────────────────────────────────────────────
function sleepHandler(): void {
    $ms = (int) ($_GET['ms'] ?? 100);
    usleep($ms * 1000);
    echo 'slept';
}

function cpuHandler(): void {
    $n = (int) ($_GET['n'] ?? 200000);
    echo sumPrimes($n);
}

// /info — ส่องว่า runtime ตั้งค่าอะไรไว้ ใช้ตอนเขียน README
function infoHandler(): void {
    $oc = opcache_get_status(false);
    echo 'php=' . PHP_VERSION
        . ' opcache=' . ($oc && $oc['opcache_enabled'] ? 'on' : 'off')
        . ' jit=' . ($oc['jit']['enabled'] ?? false ? 'on' : 'off')
        . ' memory_limit=' . ini_get('memory_limit')
        . ' workers=' . (getenv('PHP_CLI_SERVER_WORKERS') ?: '1')
        . ' pdo_pgsql=' . (extension_loaded('pdo_pgsql') ? 'yes' : 'no')
        . ' pid=' . getmypid();
}

// ─────────────────────────────────────────────────────────────
// /heavy — จำลอง framework: require ไฟล์หลายร้อยไฟล์ต่อ 1 request
//
// Laravel ของจริง autoload ไฟล์ระดับหลายร้อยถึงพันไฟล์ทุก request
// งานนี้คือ "แปลง source เป็น bytecode" ซึ่งเป็นงานเดียวที่ opcache ช่วยได้
//
// เทียบ Go: งานตรงนี้ Go ทำไปแล้วตอน `go build` จ่ายครั้งเดียวจบ
// ─────────────────────────────────────────────────────────────
function heavyHandler(): void {
    $files = glob(__DIR__ . '/vendor_sim/*.php') ?: [];
    if ($files === []) {
        http_response_code(500);
        echo 'ยังไม่ได้สร้าง vendor_sim — สั่ง: php phpdemo/gen_vendor.php 300';
        return;
    }
    foreach ($files as $f) { require_once $f; }

    $o = new Sim0();
    echo count($files) . ' ไฟล์ ' . $o->m0(1);
}

function notFound(): void {
    http_response_code(404);
    echo "ไม่มี endpoint นี้ — มี: /ping /state /db /dbp /sleep /cpu /heavy /info";
}
