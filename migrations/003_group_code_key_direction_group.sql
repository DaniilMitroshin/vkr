UPDATE students
SET group_code_key = btrim(group_code)
WHERE group_code_key IS DISTINCT FROM btrim(group_code);
