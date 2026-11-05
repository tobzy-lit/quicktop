import 'package:flutter/material.dart';
import 'services/api_service.dart';

void main() {
  runApp(const QuickTopApp());
}

class QuickTopApp extends StatelessWidget {
  const QuickTopApp({super.key});

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'QuickTop',
      debugShowCheckedModeBanner: false,
      theme: ThemeData(
        colorScheme: ColorScheme.fromSeed(seedColor: Colors.deepPurple),
        useMaterial3: true,
      ),
      home: const MainNavigationScreen(),
    );
  }
}

class MainNavigationScreen extends StatefulWidget {
  const MainNavigationScreen({super.key});

  @override
  State<MainNavigationScreen> createState() => _MainNavigationScreenState();
}

class _MainNavigationScreenState extends State<MainNavigationScreen> {
  int _currentIndex = 0;
  String? _currentUserEmail;

  void _onLoginSuccess(String email) {
    setState(() {
      _currentUserEmail = email;
      _currentIndex = 0; // Return to recharge screen upon login
    });
  }

  void _onLogout() {
    ApiService.logout();
    setState(() {
      _currentUserEmail = null;
      _currentIndex = 0;
    });
  }

  @override
  Widget build(BuildContext context) {
    final List<Widget> screens = [
      AirtimeScreen(currentUserEmail: _currentUserEmail),
      const LookupScreen(),
      _currentUserEmail == null
          ? AuthScreen(onLoginSuccess: _onLoginSuccess)
          : HistoryScreen(userEmail: _currentUserEmail!, onLogout: _onLogout),
    ];

    return Scaffold(
      appBar: AppBar(
        title: Text(
          _currentIndex == 0
              ? 'QuickTop Airtime'
              : _currentIndex == 1
                  ? 'Track Reference'
                  : (_currentUserEmail == null ? 'Sign In' : 'Account History'),
        ),
        centerTitle: true,
        backgroundColor: Theme.of(context).colorScheme.inversePrimary,
        actions: [
          if (_currentUserEmail != null)
            IconButton(
              icon: const Icon(Icons.logout),
              tooltip: 'Logout',
              onPressed: _onLogout,
            ),
        ],
      ),
      body: screens[_currentIndex],
      bottomNavigationBar: NavigationBar(
        selectedIndex: _currentIndex,
        onDestinationSelected: (index) {
          setState(() {
            _currentIndex = index;
          });
        },
        destinations: [
          const NavigationDestination(
            icon: Icon(Icons.phone_android),
            label: 'Recharge',
          ),
          const NavigationDestination(
            icon: Icon(Icons.search),
            label: 'Lookup',
          ),
          NavigationDestination(
            icon: Icon(_currentUserEmail == null
                ? Icons.person_outline
                : Icons.account_circle),
            label: _currentUserEmail == null ? 'Account' : 'History',
          ),
        ],
      ),
    );
  }
}

// ==========================================
// 1. RECHARGE SCREEN
// ==========================================
class AirtimeScreen extends StatefulWidget {
  final String? currentUserEmail;
  const AirtimeScreen({super.key, this.currentUserEmail});

  @override
  State<AirtimeScreen> createState() => _AirtimeScreenState();
}

class _AirtimeScreenState extends State<AirtimeScreen> {
  final _formKey = GlobalKey<FormState>();
  final _phoneController = TextEditingController();
  final _amountController = TextEditingController();

  List<String> _networks = [];
  String? _selectedNetwork;
  bool _isLoadingNetworks = true;
  bool _isSubmitting = false;
  String? _resultMessage;
  bool _isSuccess = false;

  @override
  void initState() {
    super.initState();
    _loadNetworks();
  }

  Future<void> _loadNetworks() async {
    try {
      final networks = await ApiService.getNetworks();
      setState(() {
        _networks = List<String>.from(networks);
        if (_networks.isNotEmpty) {
          _selectedNetwork = _networks.first;
        }
        _isLoadingNetworks = false;
      });
    } catch (e) {
      setState(() {
        _isLoadingNetworks = false;
        _resultMessage = 'Failed to load networks: $e';
        _isSuccess = false;
      });
    }
  }

  Future<void> _handlePurchase() async {
    if (!_formKey.currentState!.validate()) return;

    setState(() {
      _isSubmitting = true;
      _resultMessage = null;
    });

    final phone = _phoneController.text.trim();
    final amount = int.parse(_amountController.text.trim());
    final network = _selectedNetwork!;

    try {
      final response = await ApiService.purchaseAirtime(phone, network, amount);
      setState(() {
        _isSuccess = true;
        _resultMessage =
            'Recharge Initialized!\nRef: ${response['reference'] ?? 'Pending'}\nStatus: ${response['status'] ?? 'processing'}';
        _phoneController.clear();
        _amountController.clear();
      });
    } catch (e) {
      setState(() {
        _isSuccess = false;
        _resultMessage = e.toString().replaceAll('Exception: ', '');
      });
    } finally {
      setState(() {
        _isSubmitting = false;
      });
    }
  }

  @override
  void dispose() {
    _phoneController.dispose();
    _amountController.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return Center(
      child: SingleChildScrollView(
        padding: const EdgeInsets.all(24.0),
        child: ConstrainedBox(
          constraints: const BoxConstraints(maxWidth: 420),
          child: Card(
            elevation: 3,
            shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(16)),
            child: Padding(
              padding: const EdgeInsets.all(24.0),
              child: _isLoadingNetworks
                  ? const Center(child: CircularProgressIndicator())
                  : Form(
                      key: _formKey,
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.stretch,
                        mainAxisSize: MainAxisSize.min,
                        children: [
                          if (widget.currentUserEmail != null)
                            Padding(
                              padding: const EdgeInsets.only(bottom: 12.0),
                              child: Text(
                                'Logged in as ${widget.currentUserEmail}',
                                textAlign: TextAlign.center,
                                style: TextStyle(
                                  color: Colors.deepPurple.shade700,
                                  fontSize: 12,
                                  fontWeight: FontWeight.w600,
                                ),
                              ),
                            ),
                          Text(
                            'Quick Airtime',
                            style: Theme.of(context).textTheme.headlineSmall,
                            textAlign: TextAlign.center,
                          ),
                          const SizedBox(height: 20),
                          DropdownButtonFormField<String>(
                            value: _selectedNetwork,
                            decoration: const InputDecoration(
                              labelText: 'Network Provider',
                              border: OutlineInputBorder(),
                              prefixIcon: Icon(Icons.cell_tower),
                            ),
                            items: _networks.map((net) {
                              return DropdownMenuItem(
                                value: net,
                                child: Text(net),
                              );
                            }).toList(),
                            onChanged: (val) {
                              setState(() {
                                _selectedNetwork = val;
                              });
                            },
                          ),
                          const SizedBox(height: 16),
                          TextFormField(
                            controller: _phoneController,
                            keyboardType: TextInputType.phone,
                            decoration: const InputDecoration(
                              labelText: 'Phone Number',
                              hintText: '08012345678',
                              border: OutlineInputBorder(),
                              prefixIcon: Icon(Icons.phone_android),
                            ),
                            validator: (val) {
                              if (val == null || val.trim().isEmpty) {
                                return 'Enter a phone number';
                              }
                              if (val.trim().length < 11) {
                                return 'Enter a valid Nigerian number';
                              }
                              return null;
                            },
                          ),
                          const SizedBox(height: 16),
                          TextFormField(
                            controller: _amountController,
                            keyboardType: TextInputType.number,
                            decoration: const InputDecoration(
                              labelText: 'Amount (₦)',
                              hintText: '500',
                              border: OutlineInputBorder(),
                              prefixIcon: Icon(Icons.monetization_on_outlined),
                            ),
                            validator: (val) {
                              if (val == null || val.trim().isEmpty) {
                                return 'Enter an amount';
                              }
                              final num = int.tryParse(val.trim());
                              if (num == null || num < 50) {
                                return 'Minimum purchase is ₦50';
                              }
                              return null;
                            },
                          ),
                          const SizedBox(height: 24),
                          ElevatedButton(
                            style: ElevatedButton.styleFrom(
                              padding: const EdgeInsets.symmetric(vertical: 14),
                              backgroundColor: Theme.of(context).colorScheme.primary,
                              foregroundColor: Colors.white,
                            ),
                            onPressed: _isSubmitting ? null : _handlePurchase,
                            child: _isSubmitting
                                ? const SizedBox(
                                    height: 20,
                                    width: 20,
                                    child: CircularProgressIndicator(
                                      strokeWidth: 2,
                                      color: Colors.white,
                                    ),
                                  )
                                : const Text('Recharge Now', style: TextStyle(fontSize: 16)),
                          ),
                          const SizedBox(height: 16),
                          if (_resultMessage != null)
                            Container(
                              padding: const EdgeInsets.all(12),
                              decoration: BoxDecoration(
                                color: _isSuccess ? Colors.green.shade50 : Colors.red.shade50,
                                borderRadius: BorderRadius.circular(8),
                                border: Border.all(
                                  color: _isSuccess ? Colors.green : Colors.red,
                                ),
                              ),
                              child: Text(
                                _resultMessage!,
                                textAlign: TextAlign.center,
                                style: TextStyle(
                                  color: _isSuccess ? Colors.green.shade900 : Colors.red.shade900,
                                  fontWeight: FontWeight.w500,
                                ),
                              ),
                            ),
                        ],
                      ),
                    ),
            ),
          ),
        ),
      ),
    );
  }
}

// ==========================================
// 2. LOOKUP SCREEN
// ==========================================
class LookupScreen extends StatefulWidget {
  const LookupScreen({super.key});

  @override
  State<LookupScreen> createState() => _LookupScreenState();
}

class _LookupScreenState extends State<LookupScreen> {
  final _refController = TextEditingController();
  bool _isLoading = false;
  Map<String, dynamic>? _transactionData;
  String? _errorMessage;

  Future<void> _searchTransaction() async {
    final ref = _refController.text.trim();
    if (ref.isEmpty) return;

    setState(() {
      _isLoading = true;
      _errorMessage = null;
      _transactionData = null;
    });

    try {
      final data = await ApiService.getTransaction(ref);
      setState(() {
        _transactionData = data;
        _isLoading = false;
      });
    } catch (e) {
      setState(() {
        _errorMessage = e.toString().replaceAll('Exception: ', '');
        _isLoading = false;
      });
    }
  }

  @override
  void dispose() {
    _refController.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return Center(
      child: SingleChildScrollView(
        padding: const EdgeInsets.all(24.0),
        child: ConstrainedBox(
          constraints: const BoxConstraints(maxWidth: 420),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              Card(
                elevation: 3,
                shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(16)),
                child: Padding(
                  padding: const EdgeInsets.all(20.0),
                  child: Column(
                    children: [
                      TextField(
                        controller: _refController,
                        decoration: const InputDecoration(
                          labelText: 'Reference Number',
                          hintText: 'QT-AIR-...',
                          border: OutlineInputBorder(),
                          prefixIcon: Icon(Icons.search),
                        ),
                      ),
                      const SizedBox(height: 16),
                      ElevatedButton(
                        style: ElevatedButton.styleFrom(
                          minimumSize: const Size.fromHeight(48),
                        ),
                        onPressed: _isLoading ? null : _searchTransaction,
                        child: _isLoading
                            ? const SizedBox(
                                height: 20,
                                width: 20,
                                child: CircularProgressIndicator(strokeWidth: 2),
                              )
                            : const Text('Check Status'),
                      ),
                    ],
                  ),
                ),
              ),
              const SizedBox(height: 20),
              if (_errorMessage != null)
                Text(
                  _errorMessage!,
                  style: const TextStyle(color: Colors.red, fontWeight: FontWeight.bold),
                ),
              if (_transactionData != null)
                Card(
                  elevation: 2,
                  child: Padding(
                    padding: const EdgeInsets.all(16.0),
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        Text('Reference: ${_transactionData!['reference'] ?? ''}',
                            style: const TextStyle(fontWeight: FontWeight.bold)),
                        const Divider(),
                        Text('Status: ${_transactionData!['status'] ?? ''}'),
                        Text('Network: ${_transactionData!['network'] ?? ''}'),
                        Text('Phone: ${_transactionData!['phone_number'] ?? ''}'),
                        Text(
                          'Amount: ₦${((_transactionData!['amount'] ?? 0) / 100).toStringAsFixed(2)}',
                        ),
                      ],
                    ),
                  ),
                ),
            ],
          ),
        ),
      ),
    );
  }
}

// ==========================================
// 3. AUTH SCREEN (LOGIN / REGISTER)
// ==========================================
class AuthScreen extends StatefulWidget {
  final Function(String email) onLoginSuccess;
  const AuthScreen({super.key, required this.onLoginSuccess});

  @override
  State<AuthScreen> createState() => _AuthScreenState();
}

class _AuthScreenState extends State<AuthScreen> {
  final _formKey = GlobalKey<FormState>();
  final _nameController = TextEditingController();
  final _emailController = TextEditingController();
  final _passwordController = TextEditingController();

  bool _isLoginMode = true;
  bool _isLoading = false;
  String? _errorMessage;

  Future<void> _submit() async {
    if (!_formKey.currentState!.validate()) return;

    setState(() {
      _isLoading = true;
      _errorMessage = null;
    });

    final name = _nameController.text.trim();
    final email = _emailController.text.trim();
    final password = _passwordController.text.trim();

    try {
      if (_isLoginMode) {
        await ApiService.login(email, password);
        widget.onLoginSuccess(email);
      } else {
        await ApiService.register(name, email, password);
        await ApiService.login(email, password);
        widget.onLoginSuccess(email);
      }
    } catch (e) {
      setState(() {
        _errorMessage = e.toString().replaceAll('Exception: ', '');
      });
    } finally {
      setState(() {
        _isLoading = false;
      });
    }
  }

  @override
  void dispose() {
    _nameController.dispose();
    _emailController.dispose();
    _passwordController.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return Center(
      child: SingleChildScrollView(
        padding: const EdgeInsets.all(24.0),
        child: ConstrainedBox(
          constraints: const BoxConstraints(maxWidth: 400),
          child: Card(
            elevation: 3,
            shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(16)),
            child: Padding(
              padding: const EdgeInsets.all(24.0),
              child: Form(
                key: _formKey,
                child: Column(
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    Icon(Icons.lock_outline, size: 48, color: Theme.of(context).primaryColor),
                    const SizedBox(height: 12),
                    Text(
                      _isLoginMode ? 'Welcome Back' : 'Create Account',
                      style: Theme.of(context).textTheme.headlineSmall,
                    ),
                    const SizedBox(height: 20),
                    if (!_isLoginMode) ...[
                      TextFormField(
                        controller: _nameController,
                        decoration: const InputDecoration(
                          labelText: 'Full Name',
                          border: OutlineInputBorder(),
                          prefixIcon: Icon(Icons.person_outline),
                        ),
                        validator: (val) {
                          if (val == null || val.trim().isEmpty) {
                            return 'Enter your name';
                          }
                          return null;
                        },
                      ),
                      const SizedBox(height: 16),
                    ],
                    TextFormField(
                      controller: _emailController,
                      keyboardType: TextInputType.emailAddress,
                      decoration: const InputDecoration(
                        labelText: 'Email Address',
                        border: OutlineInputBorder(),
                        prefixIcon: Icon(Icons.email_outlined),
                      ),
                      validator: (val) {
                        if (val == null || !val.contains('@')) {
                          return 'Enter a valid email';
                        }
                        return null;
                      },
                    ),
                    const SizedBox(height: 16),
                    TextFormField(
                      controller: _passwordController,
                      obscureText: true,
                      decoration: const InputDecoration(
                        labelText: 'Password',
                        border: OutlineInputBorder(),
                        prefixIcon: Icon(Icons.lock_outline),
                      ),
                      validator: (val) {
                        if (val == null || val.length < 6) {
                          return 'Password must be at least 6 characters';
                        }
                        return null;
                      },
                    ),
                    const SizedBox(height: 24),
                    ElevatedButton(
                      style: ElevatedButton.styleFrom(
                        minimumSize: const Size.fromHeight(48),
                        backgroundColor: Theme.of(context).colorScheme.primary,
                        foregroundColor: Colors.white,
                      ),
                      onPressed: _isLoading ? null : _submit,
                      child: _isLoading
                          ? const SizedBox(
                              height: 20,
                              width: 20,
                              child: CircularProgressIndicator(
                                strokeWidth: 2,
                                color: Colors.white,
                              ),
                            )
                          : Text(_isLoginMode ? 'Login' : 'Sign Up'),
                    ),
                    const SizedBox(height: 12),
                    TextButton(
                      onPressed: () {
                        setState(() {
                          _isLoginMode = !_isLoginMode;
                          _errorMessage = null;
                        });
                      },
                      child: Text(_isLoginMode
                          ? "Don't have an account? Sign Up"
                          : 'Already registered? Login'),
                    ),
                    if (_errorMessage != null) ...[
                      const SizedBox(height: 12),
                      Text(
                        _errorMessage!,
                        textAlign: TextAlign.center,
                        style: const TextStyle(color: Colors.red, fontWeight: FontWeight.w500),
                      ),
                    ],
                  ],
                ),
              ),
            ),
          ),
        ),
      ),
    );
  }
}

// ==========================================
// 4. USER TRANSACTION HISTORY SCREEN
// ==========================================
class HistoryScreen extends StatefulWidget {
  final String userEmail;
  final VoidCallback onLogout;

  const HistoryScreen({super.key, required this.userEmail, required this.onLogout});

  @override
  State<HistoryScreen> createState() => _HistoryScreenState();
}

class _HistoryScreenState extends State<HistoryScreen> {
  bool _isLoading = true;
  List<dynamic> _transactions = [];
  String? _errorMessage;

  @override
  void initState() {
    super.initState();
    _fetchHistory();
  }

  Future<void> _fetchHistory() async {
    setState(() {
      _isLoading = true;
      _errorMessage = null;
    });

    try {
      final txs = await ApiService.getUserTransactions();
      setState(() {
        _transactions = txs;
        _isLoading = false;
      });
    } catch (e) {
      setState(() {
        _errorMessage = e.toString().replaceAll('Exception: ', '');
        _isLoading = false;
      });
    }
  }

  @override
  Widget build(BuildContext context) {
    return Center(
      child: ConstrainedBox(
        constraints: const BoxConstraints(maxWidth: 600),
        child: Padding(
          padding: const EdgeInsets.all(16.0),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              Card(
                child: Padding(
                  padding: const EdgeInsets.all(12.0),
                  child: Row(
                    children: [
                      const CircleAvatar(child: Icon(Icons.person)),
                      const SizedBox(width: 12),
                      Expanded(
                        child: Text(
                          widget.userEmail,
                          style: const TextStyle(fontWeight: FontWeight.bold),
                        ),
                      ),
                      IconButton(
                        icon: const Icon(Icons.refresh),
                        onPressed: _fetchHistory,
                        tooltip: 'Refresh',
                      ),
                    ],
                  ),
                ),
              ),
              const SizedBox(height: 12),
              Expanded(
                child: _isLoading
                    ? const Center(child: CircularProgressIndicator())
                    : _errorMessage != null
                        ? Center(
                            child: Text(
                              _errorMessage!,
                              style: const TextStyle(color: Colors.red),
                            ),
                          )
                        : _transactions.isEmpty
                            ? const Center(
                                child: Text('No transactions found for this account.'),
                              )
                            : ListView.builder(
                                itemCount: _transactions.length,
                                itemBuilder: (context, index) {
                                  final tx = _transactions[index];
                                  final status = (tx['status'] ?? 'unknown').toString().toUpperCase();
                                  final amount = ((tx['amount'] ?? 0) / 100).toStringAsFixed(2);
                                  final phone = tx['phone_number'] ?? '';
                                  final network = tx['network'] ?? '';
                                  final ref = tx['reference'] ?? '';

                                  return Card(
                                    margin: const EdgeInsets.symmetric(vertical: 6),
                                    child: ListTile(
                                      leading: CircleAvatar(
                                        backgroundColor: status == 'SUCCESS'
                                            ? Colors.green.shade100
                                            : status == 'FAILED'
                                                ? Colors.red.shade100
                                                : Colors.amber.shade100,
                                        child: Icon(
                                          status == 'SUCCESS'
                                              ? Icons.check
                                              : status == 'FAILED'
                                                  ? Icons.close
                                                  : Icons.sync,
                                          color: status == 'SUCCESS'
                                              ? Colors.green.shade800
                                              : status == 'FAILED'
                                                  ? Colors.red.shade800
                                                  : Colors.amber.shade800,
                                        ),
                                      ),
                                      title: Text('₦$amount • $network to $phone'),
                                      subtitle: Text(ref, style: const TextStyle(fontSize: 11)),
                                      trailing: Text(
                                        status,
                                        style: TextStyle(
                                          fontWeight: FontWeight.bold,
                                          fontSize: 12,
                                          color: status == 'SUCCESS'
                                              ? Colors.green.shade800
                                              : status == 'FAILED'
                                                  ? Colors.red.shade800
                                                  : Colors.amber.shade800,
                                        ),
                                      ),
                                    ),
                                  );
                                },
                              ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}