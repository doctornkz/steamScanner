CREATE TABLE "items" (
  "id_items" varchar(100) NOT NULL,
  "new_price" float DEFAULT '0',
  "old_price" float DEFAULT '0',
  "min_price" float DEFAULT '1000',
  "max_price" float DEFAULT '0',
  PRIMARY KEY ("id_items")
);
;
