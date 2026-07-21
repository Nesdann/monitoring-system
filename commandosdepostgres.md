sudo systemctl start postgresql

psql -U monitoring -d monitoring -h localhost

SELECT * FROM connections LIMIT 10;
SELECT * FROM processes LIMIT 10;
SELECT * FROM metrics LIMIT 10;
SELECT * FROM alerts LIMIT 10;

DELETE FROM connections;
DELETE FROM processes;
DELETE FROM metrics;

