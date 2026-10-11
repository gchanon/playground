-- extension ที่ demo ต้องใช้ (ทั้งหมดมาพร้อม image อยู่แล้ว แค่ต้อง CREATE)
CREATE EXTENSION IF NOT EXISTS pgstattuple;    -- นับ dead tuple แบบแม่น (demo mvcc)
CREATE EXTENSION IF NOT EXISTS pg_buffercache; -- ส่องว่าใน shared_buffers มีหน้าไหนบ้าง
CREATE EXTENSION IF NOT EXISTS pg_prewarm;     -- บังคับโหลดตารางเข้า cache
CREATE EXTENSION IF NOT EXISTS pageinspect;   -- งัดดูข้างใน page 8KB ทีละ byte (demo page)
