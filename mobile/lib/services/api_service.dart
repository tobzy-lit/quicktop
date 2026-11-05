import 'dart:convert';
import 'package:http/http.dart' as http;

class ApiService {
  static const String baseUrl = 'http://localhost:8080';

  // Holds the JWT token when the user is logged in
  static String? authToken;

  /// Check backend connectivity
  static Future checkHealth() async {
    final response = await http.get(Uri.parse('$baseUrl/health'));
    if (response.statusCode == 200) {
      return jsonDecode(response.body);
    } else {
      throw Exception('Server returned status ${response.statusCode}');
    }
  }

  /// Fetch supported telco networks
  static Future getNetworks() async {
    final response = await http.get(Uri.parse('$baseUrl/api/v1/networks'));
    if (response.statusCode == 200) {
      final data = jsonDecode(response.body);
      return List<String>.from(data['networks']);
    } else {
      throw Exception('Failed to fetch networks');
    }
  }

  /// User Registration (name, email, password)
  static Future register(String name, String email, String password) async {
    final body = jsonEncode({
      'name': name,
      'email': email,
      'password': password,
    });

    final response = await http.post(
      Uri.parse('$baseUrl/api/v1/auth/register'),
      headers: {'Content-Type': 'application/json'},
      body: body,
    );

    final data = jsonDecode(response.body);
    if (response.statusCode == 200 || response.statusCode == 201) {
      return data;
    } else {
      throw Exception(data['error'] ?? 'Registration failed');
    }
  }

  /// User Login (saves JWT token)
  static Future login(String email, String password) async {
    final body = jsonEncode({
      'email': email,
      'password': password,
    });

    final response = await http.post(
      Uri.parse('$baseUrl/api/v1/auth/login'),
      headers: {'Content-Type': 'application/json'},
      body: body,
    );

    final data = jsonDecode(response.body);
    if (response.statusCode == 200) {
      authToken = data['token'];
      return data;
    } else {
      throw Exception(data['error'] ?? 'Login failed');
    }
  }

  /// Logout
  static void logout() {
    authToken = null;
  }

  /// Purchase Airtime (optionally authenticated if authToken is set)
  static Future purchaseAirtime(String phoneNumber, String network, int amountNaira) async {
    final body = jsonEncode({
      'phone_number': phoneNumber,
      'network': network,
      'amount': amountNaira * 100, // Naira to kobo
    });

    final headers = <String, String>{
      'Content-Type': 'application/json',
    };

    if (authToken != null && authToken!.isNotEmpty) {
      headers['Authorization'] = 'Bearer $authToken';
    }

    final response = await http.post(
      Uri.parse('$baseUrl/api/v1/airtime/purchase'),
      headers: headers,
      body: body,
    );

    final data = jsonDecode(response.body);
    if (response.statusCode == 200 || response.statusCode == 201 || response.statusCode == 202) {
      return data;
    } else {
      throw Exception(data['error'] ?? 'Purchase failed (${response.statusCode})');
    }
  }

  /// Look up single transaction by reference
  static Future getTransaction(String reference) async {
    final response = await http.get(
      Uri.parse('$baseUrl/api/v1/transactions/$reference'),
    );

    final data = jsonDecode(response.body);
    if (response.statusCode == 200) {
      return data;
    } else {
      throw Exception(data['error'] ?? 'Transaction not found (${response.statusCode})');
    }
  }

  /// Fetch transaction history for authenticated user
  static Future getUserTransactions() async {
    if (authToken == null) {
      throw Exception('Authentication required');
    }

    final response = await http.get(
      Uri.parse('$baseUrl/api/v1/user/transactions'),
      headers: {
        'Authorization': 'Bearer $authToken',
      },
    );

    final data = jsonDecode(response.body);
    if (response.statusCode == 200) {
      return data['transactions'] ?? [];
    } else {
      throw Exception(data['error'] ?? 'Failed to fetch transactions');
    }
  }
}
