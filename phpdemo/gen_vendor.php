<?php
declare(strict_types=1);

// gen_vendor.php — สร้างไฟล์ PHP จำนวนมากเพื่อจำลอง vendor/ ของ framework
//
// ทำไมต้องมี: demo opcache วัดจาก srv.php ไฟล์เดียว 150 บรรทัด แล้ว "ไม่ต่างกันเลย"
// เพราะ parse ไฟล์เดียวเล็กๆ ถูกมากจนไม่มีอะไรให้แคช
// Laravel ของจริง autoload ไฟล์หลายร้อยถึงพันไฟล์ต่อ request — ตรงนั้นแหละที่ opcache ออกฤทธิ์
//
//   php phpdemo/gen_vendor.php 300

$n   = (int) ($argv[1] ?? 300);
$dir = __DIR__ . '/vendor_sim';

if (!is_dir($dir)) { mkdir($dir, 0o755, true); }
array_map('unlink', glob("$dir/*.php") ?: []);

for ($i = 0; $i < $n; $i++) {
    $methods = '';
    for ($m = 0; $m < 12; $m++) {
        $methods .= <<<PHP

            public function m$m(int \$x): int
            {
                \$acc = \$x;
                foreach ([1, 2, 3, 4, 5] as \$k) {
                    \$acc = match (true) {
                        \$k % 2 === 0 => \$acc + \$k * $i,
                        default       => \$acc - \$k,
                    };
                }
                return \$acc;
            }

        PHP;
    }

    file_put_contents("$dir/Class$i.php", <<<PHP
    <?php
    declare(strict_types=1);

    final class Sim$i
    {
        public function __construct(private readonly int \$seed = $i) {}
    $methods
    }
    PHP);
}

$bytes = array_sum(array_map('filesize', glob("$dir/*.php")));
printf("สร้าง %d ไฟล์ รวม %.1f KB ที่ %s\n", $n, $bytes / 1024, $dir);
