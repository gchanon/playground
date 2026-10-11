<?php
declare(strict_types=1);

// setup.php — เตรียมตารางที่ demo lifecycle ต้องใช้
// รันครั้งเดียวก่อนเริ่ม: php phpdemo/setup.php

require __DIR__ . '/lib.php';

$db = pdo();
$db->exec('CREATE TABLE IF NOT EXISTS phpdemo_counter(k text PRIMARY KEY, n bigint NOT NULL)');
$db->exec("INSERT INTO phpdemo_counter VALUES ('hits', 0) ON CONFLICT (k) DO UPDATE SET n = 0");
echo "เตรียมตาราง phpdemo_counter เรียบร้อย (รีเซ็ตเป็น 0)\n";
