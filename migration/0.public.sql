-- auto-generated definition
create type partition_range as
(
    week        varchar,
    begin_year  varchar,
    begin_month varchar,
    begin_day   varchar,
    begin_at    timestamp with time zone,
    end_at      timestamp with time zone
);

create function chain_calc_epoch_by_timestamp(ts timestamp with time zone) returns bigint
    immutable
    strict
    language plpgsql
as
$$
DECLARE
    baseTime TIMESTAMPTZ = TIMESTAMPTZ '2020-08-25T06:00:00+08:00';
BEGIN
    -- 根据时间计算高度
    RETURN extract(EPOCH FROM (ts - baseTime))::bigint / 30;
END
$$;

create function chain_calc_timestamp_by_epoch(height bigint) returns timestamp with time zone
    immutable
    strict
    language plpgsql
as
$$
DECLARE
    baseEpoch bigint = extract(EPOCH FROM TIMESTAMPTZ '2020-08-25T06:00:00+08:00')::bigint;
BEGIN
    -- 根据高度计算时间
    RETURN to_timestamp(baseEpoch + height * 30);
END
$$;

create function chain_partition_create(schema character varying, "table" character varying, suffix character varying, begin_epoch bigint, end_epoch bigint) returns character varying
    language plpgsql
as
$$
DECLARE
    part_table_name varchar = format('%s_%s', "table", suffix);
BEGIN
    EXECUTE format('CREATE TABLE IF NOT EXISTS %s PARTITION OF %s FOR VALUES FROM (%s) TO (%s)',
                   concat_ws('.', schema, part_table_name), concat_ws('.', schema, "table"), begin_epoch, end_epoch);

    RETURN part_table_name;
END
$$;

create procedure chain_partition_declare(schema character varying, "table" character varying, range_type character varying DEFAULT 'week'::character varying)
    language plpgsql
as
$$
DECLARE
    insert_func_name        TEXT             := format('%s.action_%s_range_insert', schema, "table");
    insert_action_statement TEXT;
    range_func_name         TEXT;
    schema_table            CHARACTER VARYING=concat_ws('.', schema, "table");
BEGIN
    IF range_type = 'day' THEN
        range_func_name = 'public.range_partition_day';
    ELSEIF range_type = 'month' THEN
        range_func_name = 'public.range_partition_month';
    ELSEIF range_type = 'year' THEN
        range_func_name = 'public.range_partition_year';
    ELSEIF range_type = 'week' THEN
        range_func_name = 'public.range_partition_week';
    ELSE
        RAISE EXCEPTION 'unsupported range type(only in day, week, month, year)';
    END IF;
    insert_action_statement := E'CREATE OR REPLACE FUNCTION ' || insert_func_name || '(table_row ' || schema_table ||
                               ') ' ||
                               'RETURNS VOID AS ' ||
                               '$body$ ' ||
                               'DECLARE ' ||
                               'schema CHARACTER VARYING = ''' || schema || ''';' ||
                               '"table" CHARACTER VARYING = ''' || "table" || '''; ' ||
                               'range_type CHARACTER VARYING = ''' || range_type || '''; ' ||
                               'ts TIMESTAMPTZ = public.chain_calc_timestamp_by_epoch(table_row.epoch); ' ||
                               'pr     public.PARTITION_RANGE = ' || range_func_name || '(ts); ' ||
                               'begin_epoch BIGINT = public.chain_calc_epoch_by_timestamp(pr.begin_at); ' ||
                               'end_epoch BIGINT = public.chain_calc_epoch_by_timestamp(pr.end_at); ' ||
                               'suffix CHARACTER VARYING; ' ||
                               'part_table_name CHARACTER VARYING; ' ||
                               'BEGIN ' ||
                               'IF begin_epoch < 0 THEN begin_epoch = 0; END IF; ' ||
                               'IF end_epoch < 0 THEN end_epoch = 0; END IF; ' ||
                               'suffix = public.chain_partition_suffix(range_type,pr,begin_epoch,end_epoch); ' ||
                               'part_table_name = public.partition_exists(schema,"table", suffix); ' ||
                               'IF part_table_name ISNULL THEN ' ||
                               'SELECT public.chain_partition_create(schema, "table", suffix, begin_epoch, end_epoch) ' ||
                               'INTO part_table_name; ' ||
                               'END IF; ' ||
                               'EXECUTE format(''INSERT INTO %s SELECT $1.*'', concat_ws(''.'',schema, part_table_name)) USING table_row; '
                                   'END $body$ LANGUAGE plpgsql';
    EXECUTE insert_action_statement;
    EXECUTE format('DROP RULE IF EXISTS range_insert_action_rule ON %s', schema_table);
    EXECUTE format('CREATE RULE range_insert_action_rule AS ON INSERT TO %s DO INSTEAD SELECT %s(new)', schema_table,
                   insert_func_name);
END ;
$$;

create function chain_partition_suffix(range_type character varying, partition_range partition_range, begin_epoch bigint, end_epoch bigint) returns character varying
    language plpgsql
as
$$
DECLARE
    suffix character varying;
BEGIN
    IF range_type = 'day' THEN
        suffix = concat_ws('_', 'day', partition_range.begin_year, partition_range.begin_month,
                           partition_range.begin_day, begin_epoch, end_epoch)::CHARACTER VARYING;
    ELSEIF range_type = 'month' THEN
        suffix = concat_ws('_', 'month', partition_range.begin_year, partition_range.begin_month, begin_epoch,
                           end_epoch)::CHARACTER VARYING;
    ELSEIF range_type = 'year' THEN
        suffix = concat_ws('_', 'year', partition_range.begin_year, begin_epoch, end_epoch)::CHARACTER VARYING;
    ELSEIF range_type = 'week' THEN
        suffix = concat_ws('_', format('w%s', partition_range.week), partition_range.begin_year,
                           partition_range.begin_month, partition_range.begin_day, begin_epoch,
                           end_epoch)::CHARACTER VARYING;
    ELSE
        RAISE EXCEPTION 'unsupported range type(only in day, week, month, year)';
    END IF;

    RETURN suffix;
END
$$;

create function describe_table(p_schema_name character varying, p_table_name character varying) returns SETOF text
    language plpgsql
as
$$
DECLARE
    v_table_ddl   text;
    column_record record;
    table_rec record;
    constraint_rec record;
    firstrec boolean;
BEGIN
    FOR table_rec IN
        SELECT c.relname, c.oid FROM pg_catalog.pg_class c
            LEFT JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
                WHERE relkind = 'r'
                AND n.nspname = p_schema_name
                AND relname~ ('^('||p_table_name||')$')
          ORDER BY c.relname
    LOOP
        FOR column_record IN
            SELECT
                b.nspname as schema_name,
                b.relname as table_name,
                a.attname as column_name,
                pg_catalog.format_type(a.atttypid, a.atttypmod) as column_type,
                CASE WHEN
                    (SELECT substring(pg_catalog.pg_get_expr(d.adbin, d.adrelid) for 128)
                     FROM pg_catalog.pg_attrdef d
                     WHERE d.adrelid = a.attrelid AND d.adnum = a.attnum AND a.atthasdef) IS NOT NULL THEN
                    'DEFAULT '|| (SELECT substring(pg_catalog.pg_get_expr(d.adbin, d.adrelid) for 128)
                                  FROM pg_catalog.pg_attrdef d
                                  WHERE d.adrelid = a.attrelid AND d.adnum = a.attnum AND a.atthasdef)
                ELSE
                    ''
                END as column_default_value,
                CASE WHEN a.attnotnull = true THEN
                    'NOT NULL'
                ELSE
                    'NULL'
                END as column_not_null,
                a.attnum as attnum,
                e.max_attnum as max_attnum
            FROM
                pg_catalog.pg_attribute a
                INNER JOIN
                 (SELECT c.oid,
                    n.nspname,
                    c.relname
                  FROM pg_catalog.pg_class c
                       LEFT JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
                  WHERE c.oid = table_rec.oid
                  ORDER BY 2, 3) b
                ON a.attrelid = b.oid
                INNER JOIN
                 (SELECT
                      a.attrelid,
                      max(a.attnum) as max_attnum
                  FROM pg_catalog.pg_attribute a
                  WHERE a.attnum > 0
                    AND NOT a.attisdropped
                  GROUP BY a.attrelid) e
                ON a.attrelid=e.attrelid
            WHERE a.attnum > 0
              AND NOT a.attisdropped
            ORDER BY a.attnum
        LOOP
            IF column_record.attnum = 1 THEN
                v_table_ddl:='CREATE TABLE '||column_record.schema_name||'.'||column_record.table_name||' (';
            ELSE
                v_table_ddl:=v_table_ddl||',';
            END IF;

            IF column_record.attnum <= column_record.max_attnum THEN
                v_table_ddl:=v_table_ddl||chr(10)||
                         '    '||column_record.column_name||' '||column_record.column_type||' '||column_record.column_default_value||' '||column_record.column_not_null;
            END IF;
        END LOOP;

        firstrec := TRUE;
        FOR constraint_rec IN
            SELECT conname, pg_get_constraintdef(c.oid) as constrainddef
                FROM pg_constraint c
                    WHERE conrelid=(
                        SELECT attrelid FROM pg_attribute
                        WHERE attrelid = (
                            SELECT oid FROM pg_class WHERE relname = table_rec.relname
                                AND relnamespace = (SELECT ns.oid FROM pg_namespace ns WHERE ns.nspname = p_schema_name)
                        ) AND attname='tableoid'
                    )
        LOOP
            v_table_ddl:=v_table_ddl||','||chr(10);
            v_table_ddl:=v_table_ddl||'CONSTRAINT '||constraint_rec.conname;
            v_table_ddl:=v_table_ddl||chr(10)||'    '||constraint_rec.constrainddef;
            firstrec := FALSE;
        END LOOP;
        v_table_ddl:=v_table_ddl||');';
        RETURN NEXT v_table_ddl;
    END LOOP;
END;
$$;

create function partition_constraint_keys(schema character varying, "table" character varying) returns character varying
    immutable
    language plpgsql
as
$$
DECLARE
    unique_keys character varying;
BEGIN
    WITH t AS (SELECT DISTINCT kcu.column_name
               FROM information_schema.table_constraints AS tc
                        JOIN information_schema.key_column_usage AS kcu ON tc.constraint_name = kcu.constraint_name
                        JOIN information_schema.constraint_column_usage AS ccu
                             ON ccu.constraint_name = tc.constraint_name
               WHERE constraint_type = 'UNIQUE'
                 AND tc.table_schema = schema
                 AND tc.table_name = "table")
    SELECT string_agg(concat(t.column_name, '=$1.', t.column_name), ' AND ')
    FROM t
    INTO unique_keys;
    RETURN unique_keys;
END
$$;

create function partition_exists(schema character varying, "table" character varying, suffix character varying) returns character varying
    immutable
    language plpgsql
as
$$
DECLARE
    part_table_name_without_schema VARCHAR;
BEGIN
    WITH o AS (SELECT inhrelid FROM pg_inherits WHERE inhparent = concat_ws('.', schema, "table")::REGCLASS)
    SELECT relname
    FROM pg_class
    WHERE oid IN (SELECT * FROM o)
      AND relname = concat_ws('_', "table", suffix)
    INTO part_table_name_without_schema;
    RETURN part_table_name_without_schema;
END
$$;

create function range_partition_create(schema character varying, "table" character varying, suffix character varying, range_type partition_range) returns character varying
    language plpgsql
as
$$
DECLARE
    part_table_name varchar = format('%s_%s', "table", suffix);
BEGIN
    EXECUTE format('CREATE TABLE IF NOT EXISTS %s PARTITION OF %s FOR VALUES FROM (''%s'') TO (''%s'')',
                   concat_ws('.', schema, part_table_name), concat_ws('.', schema, "table"), range_type.begin_at,
                   range_type.end_at);

    RETURN part_table_name;
END
$$;

create function range_partition_day(ts timestamp with time zone) returns SETOF partition_range
    immutable
    strict
    language plpgsql
as
$$
DECLARE
    ts     TIMESTAMPTZ = date_trunc('DAY', ts);
    end_at TIMESTAMPTZ = ts + INTERVAL '1 DAY';
BEGIN
    RETURN QUERY SELECT extract(WEEK FROM ts)::varchar,
                        extract(YEAR FROM ts)::varchar,
                        lpad(extract(MONTH FROM ts)::varchar, 2, '0')::varchar,
                        lpad(extract(DAY FROM ts)::varchar, 2, '0')::varchar,
                        ts,
                        end_at;
END
$$;

create procedure range_partition_declare(schema character varying, "table" character varying, range_type character varying DEFAULT 'week'::character varying, range_col character varying DEFAULT 'ts'::character varying)
    language plpgsql
as
$$
DECLARE
    insert_func_name        TEXT             := format('%s.action_%s_range_insert', schema, "table");
    insert_action_statement TEXT;
    range_func_name         TEXT;
    schema_table            CHARACTER VARYING=concat_ws('.', schema, "table");
BEGIN
    IF range_type = 'day' THEN
        range_func_name = 'public.range_partition_day';
    ELSEIF range_type = 'month' THEN
        range_func_name = 'public.range_partition_month';
    ELSEIF range_type = 'year' THEN
        range_func_name = 'public.range_partition_year';
    ELSEIF range_type = 'week' THEN
        range_func_name = 'public.range_partition_week';
    ELSE
        RAISE EXCEPTION 'unsupported range type(only in day, week, month, year)';
    END IF;
    insert_action_statement := E'CREATE OR REPLACE FUNCTION ' || insert_func_name || '(table_row ' || schema_table ||
                               ') ' ||
                               'RETURNS VOID AS ' ||
                               '$body$ ' ||
                               'DECLARE ' ||
                               'schema CHARACTER VARYING = ''' || schema || ''';' ||
                               '"table" CHARACTER VARYING = ''' || "table" || '''; ' ||
                               'range_type CHARACTER VARYING = ''' || range_type || '''; ' ||
                               'pr     public.PARTITION_RANGE = ' || range_func_name || '(table_row.' || range_col ||
                               '); ' ||
                               'suffix CHARACTER VARYING = public.range_partition_suffix(range_type,pr); ' ||
                               'part_table_name CHARACTER VARYING = public.partition_exists(schema,"table", suffix); ' ||
                               'BEGIN ' ||
                               'IF part_table_name ISNULL THEN ' ||
                               'SELECT public.range_partition_create(schema, "table", suffix, pr) ' ||
                               'INTO part_table_name; ' ||
                               'END IF; ' ||
                               'EXECUTE format(''INSERT INTO %s SELECT $1.*'', concat_ws(''.'',schema, part_table_name)) USING table_row; '
                                   'END $body$ LANGUAGE plpgsql';
    EXECUTE insert_action_statement;
    EXECUTE format('DROP RULE IF EXISTS range_insert_action_rule ON %s', schema_table);
    EXECUTE format('CREATE RULE range_insert_action_rule AS ON INSERT TO %s DO INSTEAD SELECT %s(new)', schema_table,
                   insert_func_name);
END ;
$$;

create function range_partition_month(ts timestamp with time zone) returns SETOF partition_range
    immutable
    strict
    language plpgsql
as
$$
DECLARE
    ts     TIMESTAMPTZ = date_trunc('MONTH', ts);
    end_at TIMESTAMPTZ = ts + INTERVAL '1 MONTH';
BEGIN
    RETURN QUERY SELECT extract(WEEK FROM ts)::varchar,
                        extract(YEAR FROM ts)::varchar,
                        lpad(extract(MONTH FROM ts)::varchar, 2, '0')::varchar,
                        lpad(extract(DAY FROM ts)::varchar, 2, '0')::varchar,
                        ts,
                        end_at;
END
$$;

create function range_partition_suffix(range_type character varying, partition_range partition_range) returns character varying
    language plpgsql
as
$$
DECLARE
    suffix character varying;
BEGIN


    IF range_type = 'day' THEN
        suffix = concat_ws('_', 'day', partition_range.begin_year, partition_range.begin_month,
                           partition_range.begin_day)::CHARACTER VARYING;
    ELSEIF range_type = 'month' THEN
        suffix = concat_ws('_', 'month', partition_range.begin_year, partition_range.begin_month)::CHARACTER VARYING;
    ELSEIF range_type = 'year' THEN
        suffix = concat_ws('_', 'year', partition_range.begin_year)::CHARACTER VARYING;
    ELSEIF range_type = 'week' THEN
        suffix = concat_ws('_', format('w%s', partition_range.week), partition_range.begin_year,
                           partition_range.begin_month, partition_range.begin_day)::CHARACTER VARYING;
    ELSE
        RAISE EXCEPTION 'unsupported range type(only in day, week, month, year)';
    END IF;

    RETURN suffix;
END
$$;

create function range_partition_week(ts timestamp with time zone) returns SETOF partition_range
    immutable
    strict
    language plpgsql
as
$$
DECLARE
    ts     TIMESTAMPTZ = date_trunc('WEEK', ts);
    end_at TIMESTAMPTZ = ts + INTERVAL '1 WEEK';
BEGIN
    RETURN QUERY SELECT extract(WEEK FROM ts)::varchar,
                        extract(YEAR FROM ts)::varchar,
                        lpad(extract(MONTH FROM ts)::varchar, 2, '0')::varchar,
                        lpad(extract(DAY FROM ts)::varchar, 2, '0')::varchar,
                        ts,
                        end_at;
END
$$;

create function range_partition_year(ts timestamp with time zone) returns SETOF partition_range
    immutable
    strict
    language plpgsql
as
$$
DECLARE
    ts     TIMESTAMPTZ = date_trunc('YEAR', ts);
    end_at TIMESTAMPTZ = ts + INTERVAL '1 YEAR';
BEGIN
    RETURN QUERY SELECT extract(WEEK FROM ts)::varchar,
                        extract(YEAR FROM ts)::varchar,
                        lpad(extract(MONTH FROM ts)::varchar, 2, '0')::varchar,
                        lpad(extract(DAY FROM ts)::varchar, 2, '0')::varchar,
                        ts,
                        end_at;
END
$$;