<?php
declare(strict_types=1);

// demo.php — demo ที่รันจบในโปรเซสเดียว ไม่ต้องเปิด server
//
//   php phpdemo/demo.php                 # รันทุก demo
//   php phpdemo/demo.php -demo value     # รันทีละตัว
//
// สามตัวนี้คือ "สิ่งที่คน Go จะพลาด" เรียงตามความถี่ที่พลาดจริง
//
//   syntax  อ่าน PHP ให้ออกภายใน 20 นาที โดยใช้ Go เป็นจุดอ้างอิง
//   value   array ของ PHP เป็น value ไม่ใช่ reference — ต่างจาก slice คนละขั้ว
//   memory  ทำไม batch job ใน PHP ถึงพังตอนข้อมูลเยอะ และทางออกคืออะไร

require __DIR__ . '/lib.php';

// ---------- class ที่ demo syntax ใช้ ----------

interface Quote {
    public function price(): int;
}

// readonly = ตั้งค่าได้ครั้งเดียวตอนสร้าง แก้ไม่ได้อีก (PHP 8.1+)
// constructor property promotion: เขียน private int $amount ใน parameter
// = ประกาศ property + รับค่า + assign ให้เลยในบรรทัดเดียว
final class FixedQuote implements Quote
{
    public function __construct(private readonly int $amount) {}

    public function price(): int { return $this->amount; }
}

final class Counter
{
    public int $n = 0;
}

$demo = argFlag($argv, 'demo', 'all');

// demo memory ต้องการแรมมากกว่า 128M ที่เป็นค่า default
// ini_set แก้ค่าได้ตอน runtime — ซึ่งก็เป็นบทเรียนในตัวมันเอง:
// memory_limit ไม่ใช่กำแพงของ OS แต่เป็นเพดานที่ PHP ตั้งให้ตัวเองต่อ 1 การรัน
// (บน prod ไม่ควรแก้ตรงนี้ ควรตั้งใน php.ini หรือ pool config ของ php-fpm)
$limitWas = ini_get('memory_limit');
ini_set('memory_limit', '1G');

$run = function (string $name, callable $f) use ($demo): void {
    if ($demo === 'all' || $demo === $name) { $f(); }
};

printf("\n=== PHP %s | memory_limit=%s (php.ini ตั้งไว้ %s) | opcache(cli)=%s ===\n",
    PHP_VERSION,
    ini_get('memory_limit'),
    $limitWas,
    (opcache_get_status(false)['opcache_enabled'] ?? false) ? 'on' : 'off');

$run('syntax', demoSyntax(...));
$run('value',  demoValue(...));
$run('memory', demoMemory(...));

echo "\n";


// ═══════════════════════════════════════════════════════════════
// 1 · syntax — Go → PHP สำหรับคนที่อ่าน Go ออกอยู่แล้ว
//
// ไม่ใช่ tutorial ครบทุกเรื่อง แต่เป็น "ตารางแปล" เฉพาะที่ต้องใช้ตอนอ่านโค้ดจริง
// ═══════════════════════════════════════════════════════════════
function demoSyntax(): void
{
    header_('1 · อ่าน PHP ให้ออกด้วยความรู้ Go ที่มีอยู่');

    section('ตัวแปรกับฟังก์ชัน');
    // Go : func add(a, b int) int { return a + b }
    // PHP: ชื่อตัวแปรขึ้นต้นด้วย $ เสมอ · type เขียนไว้ "หน้า" ชื่อ (ตรงข้าม Go)
    $add = function (int $a, int $b): int { return $a + $b; };
    note('$add(2,3) = ' . $add(2, 3));
    note('type เขียนหน้าชื่อ (int $a) ตรงข้ามกับ Go (a int)');
    note('ตัวแปรต้องมี $ นำหน้าเสมอ — $a ไม่ใช่ a');

    section('array = slice และ map รวมร่างเป็นตัวเดียว');
    // PHP มี type เดียวชื่อ array ทำหน้าที่ทั้ง []T และ map[K]V
    // ข้างในจริงๆ คือ "ordered hash map" เสมอ แม้ตอนใช้เป็น list
    $list = [10, 20, 30];                       // Go: []int{10,20,30}
    $dict = ['a' => 1, 'b' => 2];               // Go: map[string]int{...}
    note('$list[1] = ' . $list[1] . '   (key คือ 0,1,2 ที่ PHP ใส่ให้เอง)');
    note('$dict["b"] = ' . $dict['b']);
    note('count($list) = ' . count($list) . '   เทียบ len() ของ Go');

    // foreach = for...range
    $out = [];
    foreach ($dict as $k => $v) { $out[] = "$k=$v"; }
    note('foreach: ' . implode(' ', $out) . '   เทียบ for k, v := range');

    // "$k=$v" ในเครื่องหมายคำพูดคู่ = PHP แทนค่าตัวแปรให้ (Go ไม่มี ต้องใช้ Sprintf)
    note('string ในเครื่องหมาย " แทนค่าตัวแปรให้เลย — ใน \' ไม่แทน');

    section('เทียบค่า: === คือสิ่งที่คุณคิดว่า == ควรเป็น');
    // นี่คือกับดักอันดับหนึ่งของคนมาจากภาษาที่ type แข็ง
    $pairs = [
        ["'0'  กับ  0",     '0',  0],
        ["'abc' กับ 0",     'abc', 0],
        ['null กับ false',  null, false],
        ['[]   กับ false',  [],   false],
        ["'1'  กับ  1",     '1',  1],
        ["'1e2' กับ '100'", '1e2', '100'],
    ];
    note(padr('เทียบอะไร', 16) . '│ ' . padr('==', 7) . '│ ===');
    note(str_repeat('─', 34));
    foreach ($pairs as [$label, $a, $b]) {
        note(padr($label, 16)
            . '│ ' . padr($a == $b ? 'true' : 'false', 7)
            . '│ ' . ($a === $b ? 'true' : 'false'));
    }
    note('กฎเดียวที่ต้องจำ: ใช้ === เสมอ ยกเว้นรู้ตัวว่ากำลังทำอะไรอยู่');

    section('class / interface — ใกล้ Go มากกว่าที่คิด');
    note('interface + method คล้าย Go แต่ต้องประกาศ implements ชัดเจน');
    note('PHP ไม่มี structural typing แบบ Go — ต้องบอกว่า implements ตัวไหน');
    $q = new FixedQuote(100);
    note('FixedQuote->price() = ' . $q->price() . '   (นิยาม class อยู่หัวไฟล์)');

    section('error handling — exception ไม่ใช่ error value');
    // Go : v, err := f(); if err != nil { ... }
    // PHP: throw / try-catch — error เดินทางขึ้นเองจนกว่าจะมีคนรับ
    try {
        throw new RuntimeException('ของจริงพัง');
    } catch (RuntimeException $e) {
        note('catch ได้: ' . $e->getMessage());
    } finally {
        note('finally ทำงานเสมอ — เทียบ defer ของ Go ได้คร่าวๆ');
    }
    note('ต่างจาก Go: error ไม่ได้อยู่ใน signature ดูจากหน้าฟังก์ชันไม่รู้ว่าพังได้มั้ย');
    note('→ ตอนอ่านโค้ด PHP ต้องไล่ดู throw เอาเอง หรือดู @throws ใน docblock');

    section('null');
    $arr = ['a' => 1];
    note('$arr["ไม่มี"] ?? "ค่าสำรอง" = ' . ($arr['ไม่มี'] ?? 'ค่าสำรอง') . '   (?? คือ null coalescing)');
    note('?-> เรียก method ได้โดยไม่พังถ้าเป็น null: $obj?->method()');
    note('Go ใช้ zero value, PHP ใช้ null — ตัวแปรที่ไม่ได้ set มีค่า null ไม่ใช่ 0');
}


// ═══════════════════════════════════════════════════════════════
// 2 · value — array เป็น value ไม่ใช่ reference
//
// นี่คือจุดที่ Go dev เขียนบั๊กโดยไม่รู้ตัว เพราะมันกลับด้านกับ Go พอดี
//
//   Go  : slice ส่งเข้าฟังก์ชัน = แชร์ array ข้างใน → แก้ในฟังก์ชัน เห็นข้างนอกด้วย
//   PHP : array ส่งเข้าฟังก์ชัน = ได้สำเนา        → แก้ในฟังก์ชัน ข้างนอกไม่รู้เรื่อง
//
//   Go  : struct ส่งเข้าฟังก์ชัน = ได้สำเนา        → แก้แล้วข้างนอกไม่เห็น
//   PHP : object ส่งเข้าฟังก์ชัน = แชร์ตัวเดียวกัน → แก้แล้วข้างนอกเห็น
//
// สลับกันเป๊ะทั้งสองฝั่ง
// ═══════════════════════════════════════════════════════════════
function demoValue(): void
{
    header_('2 · array เป็นสำเนา object เป็นตัวเดียวกัน — กลับด้านกับ Go');

    section('array: ส่งเข้าฟังก์ชันแล้วแก้ ข้างนอกไม่เปลี่ยน');
    $a = [1, 2, 3];
    (function (array $x) { $x[0] = 999; })($a);
    note('หลังเรียกฟังก์ชันที่แก้ $x[0]=999 → $a[0] ยังเป็น ' . $a[0]);
    note('เทียบ Go: ถ้าเป็น []int{1,2,3} ส่งเข้าไปแก้ ข้างนอกจะเห็น 999');

    section('object: ส่งเข้าฟังก์ชันแล้วแก้ ข้างนอกเปลี่ยนตาม');
    $o = new Counter();
    (function (Counter $c) { $c->n = 999; })($o);
    note('หลังเรียกฟังก์ชันที่แก้ $c->n=999 → $o->n เป็น ' . $o->n);
    note('เทียบ Go: ถ้าเป็น struct ส่งเข้าไปแก้ ข้างนอกจะไม่เห็น (ต้องส่ง pointer)');
    note('PHP ส่ง "object handle" ไปเสมอ — คล้ายส่ง *T ใน Go ทุกครั้งโดยอัตโนมัติ');

    section('copy-on-write: สำเนาที่ยังไม่ได้แก้ ไม่กินแรมเพิ่ม');
    // PHP ไม่ได้ก๊อป array ทันทีที่ assign — รอจนมีคนแก้ค่อยก๊อปจริง
    // วัดได้ ไม่ต้องเชื่อ
    $big = range(1, 200_000);
    note('array ต้นฉบับ 200,000 ตัว กิน ' . fmtBytes(memory_get_usage()));

    // closure ต้องคืน $copy ออกมา ไม่งั้นมันถูกเก็บทิ้งก่อนเราวัด
    [$d1, , ] = memIt(function () use ($big) { $copy = $big; return $copy; });
    note('assign เฉยๆ (ยังไม่แก้)   → แรมเพิ่ม ' . fmtBytes($d1) . '   ← ยังไม่ก๊อป แค่เพิ่ม refcount');

    [$d2, , ] = memIt(function () use ($big) { $copy = $big; $copy[0] = -1; return $copy; });
    note('assign แล้วแก้ 1 ช่อง     → แรมเพิ่ม ' . fmtBytes($d2) . '   ← ก๊อปทั้งก้อนตรงนี้');

    note('');
    note('แก้ช่องเดียว แต่ต้องก๊อปทั้งก้อน — นี่คือราคาของ value semantics');
    note('"copy-on-write" = ก๊อปตอนเขียน ไม่ใช่ตอน assign');
    note('ถ้าไม่อยากให้ก๊อป ต้องใส่ & (reference) เอง: function f(array &$x)');
    note('หรือใช้ object แทน array ซึ่งไม่เคยก๊อป');

    section('ของจริงที่เจอบ่อย: แก้ array ใน foreach');
    $rows = [['n' => 1], ['n' => 2]];
    foreach ($rows as $r) { $r['n'] *= 10; }          // ไม่มีผล — $r เป็นสำเนา
    note('foreach ธรรมดาแล้วแก้ $r → ' . json_encode(array_column($rows, 'n')) . ' (ไม่เปลี่ยน)');

    foreach ($rows as &$r) { $r['n'] *= 10; }         // มีผล — &$r คือตัวจริง
    unset($r);                                        // ต้อง unset ไม่งั้นรอบถัดไปพัง
    note('foreach แบบ &$r แล้วแก้    → ' . json_encode(array_column($rows, 'n')) . ' (เปลี่ยน)');
    note('อย่าลืม unset($r) หลัง foreach ที่ใช้ & — ไม่งั้นลูปถัดไปจะเขียนทับช่องสุดท้าย');
}


// ═══════════════════════════════════════════════════════════════
// 3 · memory — ทำไม batch job PHP ถึงพังตอนข้อมูลเยอะ
//
// PHP มี memory_limit ต่อ "การรันหนึ่งครั้ง" (คือต่อ request)
// เกินเมื่อไหร่ = fatal error ตาย ไม่ใช่ช้าลง
//
// Go ไม่มีเพดานแบบนี้ ใช้จนเครื่องหมดแล้วค่อยโดน OOM killer
// ═══════════════════════════════════════════════════════════════
function demoMemory(): void
{
    header_('3 · แรม — array ของ PHP แพงกว่า slice ของ Go มาก');

    $n = 1_000_000;

    section('ยัดเลข ' . number_format($n) . ' ตัวใส่ array');
    [$used, , $arr] = memIt(fn() => range(1, $n));          // คืน array ออกมาถือไว้ตอนวัด
    note('PHP array  ' . number_format(count($arr)) . ' ตัว = ' . fmtBytes($used));
    note('Go  []int  ' . number_format($n) . ' ตัว = ' . fmtBytes($n * 8) . ' (8 byte ต่อตัว เป๊ะ)');
    note(sprintf('ต่างกัน %.1f เท่า', $used / ($n * 8)));

    // ข้างบนคือ "packed array" — key เป็น 0,1,2,... ติดกัน PHP เก็บเป็นแถวเรียงเฉยๆ
    // พอ key ไม่ติดกันหรือเป็น string จะกลายเป็น hash เต็มรูป ต้องมี Bucket ต่อช่อง
    [$hashed, , $h] = memIt(function () use ($n) {
        $m = [];
        for ($i = 0; $i < $n; $i++) { $m["k$i"] = $i; }
        return $m;
    });
    note('PHP array key เป็น string  = ' . fmtBytes($hashed)
        . sprintf('  (%.1f เท่าของ packed)', $hashed / $used));
    unset($h);
    note('');
    note('เพราะ PHP array = hash map เสมอ ทุกช่องต้องเก็บ bucket + zval ไม่ใช่แค่ค่า');
    note('ตั้งแต่ PHP 7 มี "packed array" ที่ประหยัดขึ้นมากถ้า key เป็น 0,1,2,... ติดกัน');
    note('แต่พอ unset ตรงกลางหรือใส่ key เป็น string จะกลายเป็น hash เต็มรูปทันที');

    section('เพดาน memory_limit ชนแล้วตาย ไม่ใช่ช้าลง');
    note('ตอนนี้ตั้งไว้ ' . ini_get('memory_limit') . ' ต่อ 1 request (demo นี้ ini_set ขึ้นมาเอง)');
    note('ใช้ไปแล้ว ' . fmtBytes(memory_get_usage()) . ' · สูงสุดที่เคยแตะ ' . fmtBytes(memory_get_peak_usage()));
    note('ชนเพดาน = Fatal error: Allowed memory size exhausted → request ตายทันที');

    section('ทางออก: generator (yield) — เทียบ channel ของ Go ได้ตรงๆ');
    // generator ส่งค่าทีละตัว ไม่เคยถือทั้งก้อนไว้ในแรม
    // คนละอย่างกับ channel ตรงที่ไม่มี goroutine — มันแค่หยุดค้างแล้วเดินต่อใน thread เดิม
    unset($arr);                                           // คืนแรมก่อน จะได้วัด generator สะอาดๆ
    [$genMem, $genPeak, $sum] = memIt(function () use ($n) {
        $total = 0;
        foreach (countTo($n) as $v) { $total += $v; }
        return $total;
    });
    note('วนด้วย generator ' . number_format($n) . ' รอบ');
    note('   แรมที่ค้างอยู่  = ' . fmtBytes($genMem));
    note('   แรมที่แตะสูงสุด = ' . fmtBytes($genPeak) . '   ← ไม่เคยถือทั้งก้อนเลย');
    note('   ผลรวม ' . number_format($sum));
    note('');
    note('อุปมา: array = ขนของทั้งโกดังขึ้นรถก่อนออกเดินทาง');
    note('       generator = ขนทีละกล่อง ส่งเสร็จค่อยกลับมาเอากล่องถัดไป');
    note('');
    note('เทียบ Go: generator ใกล้ iterator (range-over-func ของ Go 1.23) มากกว่า channel');
    note('          เพราะไม่มี goroutine ไม่มีการสลับ thread — แค่หยุดค้างตรงบรรทัด yield');
    note('ของจริง: อ่าน DB ด้วย fetch() ทีละแถวแทน fetchAll() ก็หลักการเดียวกัน');
}

// countTo = generator ง่ายๆ ที่ไม่เคยสร้าง array
// สังเกต yield — ฟังก์ชันที่มี yield จะกลายเป็น Generator อัตโนมัติ
function countTo(int $n): Generator
{
    for ($i = 1; $i <= $n; $i++) { yield $i; }
}
