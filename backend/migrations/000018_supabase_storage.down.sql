-- Migration 000018 down
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.schemata WHERE schema_name = 'storage') THEN
        DROP POLICY IF EXISTS "Allow upload to shopswift" ON storage.objects;
        DROP POLICY IF EXISTS "Public Access to shopswift bucket" ON storage.objects;
        DELETE FROM storage.buckets WHERE id = 'shopswift';
    END IF;
END $$;
