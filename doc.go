// Package amqp defines the generic interfaces required in order to perform requests against a AMQP broker
// These interfaces only have functions which relate to operations expected by the AMQP protocol, which means
// technically we should be able to add a new AMQP broker type and this library will be able to use it once we have
// made the bindings necessary to implement the interface.
//
// This package does not know or care about anything outside the AMQP realm, i.e. it does not know about any additional
// plugins or tools a broker may provide to aid in execution. For example this package knows nothing about the RabbitMQ
// management API or any of the additional plugins which are unique to that broker.
//
// They are expected to be implemented separately.
//
// The only one current implementation provided at the time of writing is:
// - rabbitmq (github.com/jacklaaa89/amqp/rabbitmq)
package amqp
