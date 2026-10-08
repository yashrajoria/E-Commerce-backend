-- Migration 000018: Supabase Storage Bucket & Public Access Policies
-- Ensures the 'shopswift' bucket exists for product image assets

DO $$
BEGIN
    -- Check if storage schema exists (standard in Supabase)
    IF EXISTS (SELECT 1 FROM information_schema.schemata WHERE schema_name = 'storage') THEN
        INSERT INTO storage.buckets (id, name, public, file_size_limit, allowed_mime_types)
        VALUES (
            'shopswift',
            'shopswift',
            true,
            10485760, -- 10MB limit
            ARRAY['image/jpeg', 'image/png', 'image/webp', 'image/gif']::text[]
        )
        ON CONFLICT (id) DO UPDATE SET
            public = true,
            file_size_limit = 10485760,
            allowed_mime_types = ARRAY['image/jpeg', 'image/png', 'image/webp', 'image/gif']::text[];

        -- Allow public read access to shopswift bucket
        IF NOT EXISTS (
            SELECT 1 FROM pg_policies 
            WHERE schemaname = 'storage' AND tablename = 'objects' AND policyname = 'Public Access to shopswift bucket'
        ) THEN
            CREATE POLICY "Public Access to shopswift bucket"
            ON storage.objects FOR SELECT
            USING (bucket_id = 'shopswift');
        END IF;

        -- Allow authenticated and service upload to shopswift
        IF NOT EXISTS (
            SELECT 1 FROM pg_policies 
            WHERE schemaname = 'storage' AND tablename = 'objects' AND policyname = 'Allow upload to shopswift'
        ) THEN
            CREATE POLICY "Allow upload to shopswift"
            ON storage.objects FOR INSERT
            WITH CHECK (bucket_id = 'shopswift');
        END IF;
    END IF;
END $$;
