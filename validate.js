const YAML = require('yaml');
const schema = require("fs").readFileSync("schema.yml", { encoding: "utf8" });
var OpenAPISchemaValidator = require('openapi-schema-validator').default;

const validator = new OpenAPISchemaValidator({ version: 3 });
const result = validator.validate(YAML.parse(schema));
console.log(result);
process.exit(result.errors.length ? 1 : 0);
