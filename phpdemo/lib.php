<?php
declare(strict_types=1);

// lib.php — ตัวช่วยเล็กๆ ที่ใช้ร่วมกันทุก demo
//
// เทียบกับ pgdemo/util.go: ทำหน้าที่เดียวกัน (จัดรูปผล + วัด) แค่คนละภาษา
// อ่านไฟล์นี้คู่กับ util.go จะเห็นว่า "โค้ดแบบเดียวกัน" ใน PHP หน้าตาเป็นยังไง
//
//   declare(strict_types=1)  ต้องมีบรรทัดแรกของทุกไฟล์
//                            ไม่มี = PHP จะแปลง "5" เป็น 5 ให้เองเงียบๆ เวลาส่งเข้า int
//                            มี    = โยน TypeError เหมือน Go ที่คอมไพล์ไม่ผ่าน

// ---------- จัดรูป (ก๊อปสไตล์มาจาก util.go ตรงๆ) ----------

function header_(string $s): void { printf("\n\n══════ %s ══════\n", $s); }

function section(string $s): void { printf("\n── %s\n", $s); }

function note(string $s = ''): void {
    if ($s === '') { echo "\n"; return; }
    printf("   » %s\n", $s);
}

// padr เติมช่องว่างท้ายสตริงให้ครบ $w "ตัวอักษร"
// printf("%-14s") ของ PHP นับเป็น byte ไม่ใช่ตัวอักษร
// ภาษาไทย 1 ตัว = 3 byte ใน UTF-8 → ตารางจะเบี้ยวทันทีถ้าใช้ printf ตรงๆ
// (Go ก็นับ byte เหมือนกัน ปัญหานี้เลยหน้าตาคุ้นอยู่แล้ว)
function padr(string $s, int $w): string {
    $len = mb_strlen($s);
    return $len >= $w ? $s : $s . str_repeat(' ', $w - $len);
}

function fmtBytes(int $n): string {
    return match (true) {
        $n >= 1 << 20 => sprintf('%.1f MB', $n / (1 << 20)),
        $n >= 1 << 10 => sprintf('%.0f KB', $n / (1 << 10)),
        default       => sprintf('%d B', $n),
    };
}

function fmtDur(float $sec): string {
    return match (true) {
        $sec >= 1      => sprintf('%.2f s', $sec),
        $sec >= 1e-3   => sprintf('%.1f ms', $sec * 1e3),
        default        => sprintf('%.0f µs', $sec * 1e6),
    };
}

// ---------- วัด ----------

// timeIt รัน $f แล้วคืนเวลาที่ใช้ (วินาที)
// hrtime(true) = นาฬิกาความละเอียดระดับ ns เทียบได้กับ time.Now() ของ Go
// microtime() ความละเอียดต่ำกว่าและโดนปรับเวลาระบบได้ — อย่าใช้วัด
function timeIt(callable $f): float {
    $t0 = hrtime(true);
    $f();
    return (hrtime(true) - $t0) / 1e9;
}

// memIt วัดว่า $f ทำให้แรมที่ "ใช้อยู่จริง" เพิ่มขึ้นเท่าไหร่
//
// memory_get_usage(true)  = ถามจาก OS (จำนวนที่ allocator ขอมา) — หยาบ ขยับทีละ chunk
// memory_get_usage(false) = ที่ PHP จองให้ตัวแปรจริงๆ — ละเอียดกว่า ใช้ตัวนี้
//
// กับดักที่เพิ่งเจอตอนเขียน demo นี้: $f ต้อง "คืนของที่สร้าง" ออกมา
// ถ้า $f สร้าง array ใหญ่แล้วคืนแค่ count() ของมัน array จะถูกเก็บทิ้งตอน return
// วัดได้ 0 ทุกครั้ง ทั้งที่ระหว่างทางใช้ไปหลาย MB
// → เก็บ peak ไว้ด้วย จะได้เห็นทั้ง "ค้างอยู่เท่าไหร่" และ "เคยแตะสูงสุดเท่าไหร่"
function memIt(callable $f): array {
    gc_collect_cycles();
    $before     = memory_get_usage();
    $peakBefore = memory_get_peak_usage();
    $ret        = $f();                      // $ret ยังถือของอยู่ตอนบรรทัดถัดไป
    $after      = memory_get_usage();
    $peakAfter  = memory_get_peak_usage();
    return [$after - $before, max(0, $peakAfter - $peakBefore), $ret];
}

// ---------- DB ----------

// ต่อไปที่ PG ตัวเดียวกับ pgdemo — ไม่ต้องเปิด container ใหม่
const DSN  = 'pgsql:host=127.0.0.1;port=55432;dbname=pgdemo';
const DBU  = 'postgres';
const DBP  = 'pgdemo';

// pdo() สร้าง connection "ใหม่ทุกครั้งที่เรียก"
//
// นี่ไม่ใช่ความขี้เกียจ แต่เป็นข้อจำกัดของ PHP จริงๆ:
// PHP ไม่มี connection pool ระดับภาษา เพราะไม่มีโปรเซสที่อยู่ยาวให้ถือ pool ไว้
// (Go มี เพราะ server เป็นโปรเซสเดียวที่อยู่ตลอด)
//
// $persistent = true จะบอก PHP ว่า "เก็บ connection นี้ไว้กับ worker process"
// ครั้งหน้าที่ worker ตัวนี้รับงาน จะได้เส้นเดิมกลับมาแทนที่จะต่อใหม่
// → ใกล้เคียง pool ที่สุดที่ PHP ทำได้ แต่มีกับดัก อ่านใน README
function pdo(bool $persistent = false): PDO {
    return new PDO(DSN, DBU, DBP, [
        PDO::ATTR_ERRMODE            => PDO::ERRMODE_EXCEPTION,
        PDO::ATTR_EMULATE_PREPARES   => false,
        PDO::ATTR_PERSISTENT         => $persistent,
    ]);
}

// ---------- งานจำลอง ----------

// sumPrimes ต้องเขียนให้เหมือน gosrv/main.go บรรทัดต่อบรรทัด
// ไม่งั้นเทียบความเร็ว PHP กับ Go ไม่ได้
function sumPrimes(int $n): int {
    $sum = 0;
    for ($i = 2; $i < $n; $i++) {
        $isPrime = true;
        for ($j = 2; $j * $j <= $i; $j++) {
            if ($i % $j === 0) { $isPrime = false; break; }
        }
        if ($isPrime) { $sum += $i; }
    }
    return $sum;
}

// argFlag อ่าน flag สไตล์ Go (-demo value) จาก $argv
// PHP มี getopt() ให้ แต่มันรับแค่ -d value หรือ --demo=value
// ทำเองเพื่อให้สั่งงานเหมือน go run ./pgdemo -demo mvcc
function argFlag(array $argv, string $name, string $default): string {
    foreach ($argv as $i => $a) {
        if ($a === "-$name" || $a === "--$name") {
            return $argv[$i + 1] ?? $default;
        }
        if (str_starts_with($a, "-$name=")) {
            return substr($a, strlen($name) + 2);
        }
    }
    return $default;
}
