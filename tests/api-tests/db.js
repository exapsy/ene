// MongoDB Migration Script for API Tests
// This script initializes the database with test data

// Switch to the test database
db = db.getSiblingDB('test');

// Create a collection for API health check data
db.createCollection('health_status');

// Insert initial health status document
db.health_status.insertOne({
  service: 'api',
  status: 'healthy',
  last_check: new Date(),
  version: '1.0.0',
  environment: 'test'
});

// Create a users collection for testing
db.createCollection('users');

// Insert test users
db.users.insertMany([
  {
    username: 'testuser1',
    email: 'test1@example.com',
    created_at: new Date(),
    active: true
  },
  {
    username: 'testuser2',
    email: 'test2@example.com',
    created_at: new Date(),
    active: true
  }
]);

// Create indexes for better query performance
db.users.createIndex({ username: 1 }, { unique: true });
db.users.createIndex({ email: 1 }, { unique: true });

// Create a sample products collection
db.createCollection('products');

db.products.insertMany([
  {
    name: 'Product A',
    price: 29.99,
    stock: 100,
    category: 'electronics',
    created_at: new Date()
  },
  {
    name: 'Product B',
    price: 49.99,
    stock: 50,
    category: 'electronics',
    created_at: new Date()
  },
  {
    name: 'Product C',
    price: 19.99,
    stock: 200,
    category: 'accessories',
    created_at: new Date()
  }
]);

// Create indexes on products
db.products.createIndex({ name: 1 });
db.products.createIndex({ category: 1 });

print('✅ Database migration completed successfully');
print('📊 Collections created: health_status, users, products');
print('👥 Users inserted: ' + db.users.countDocuments());
print('📦 Products inserted: ' + db.products.countDocuments());
